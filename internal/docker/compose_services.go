package docker

// compose_services.go — resolved per-service facts and the per-service health
// gate used by the rolling deploy (D17, D-0041).
//
// Purpose: read what the resolved compose model says about each service and
// wait for a restarted service to reach the state its kind promises. The
// manifest compose (every -f file plus every --env-file) is the only input, so
// plugin fragments and their computed variables count, exactly as for restart.
// Constraints: every docker call goes through this package (the docker
// funnel); the gate polls through an injectable Clock, never a bare sleep, so
// a remote host can drive the same gate with its own state source (P7-DEPL-24).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Gate timeouts by service kind (D17): healthcheck or one-shot exit, else running.
const (
	HealthyTimeout = 60 * time.Second
	RunningTimeout = 30 * time.Second
	gatePollEvery  = 2 * time.Second
)

// ServiceSpec is what the resolved compose model says about one service.
type ServiceSpec struct {
	Name           string `json:"name"`
	HasHealthcheck bool   `json:"has_healthcheck"`
	Restart        string `json:"restart"`
}

// OneShot reports a service that is meant to run once and exit: restart "no"
// and no healthcheck. A declared healthcheck wins (D17 order): such a service
// is long-running and is gated on healthy, never on exit.
func (s ServiceSpec) OneShot() bool { return s.Restart == "no" && !s.HasHealthcheck }

// ComposeServiceNames returns `docker compose config --services` over the
// manifest and env files, in docker's order.
func (c *Compose) ComposeServiceNames(ctx context.Context, workdir string) ([]string, error) {
	out, err := c.capture(ctx, workdir, "config", "--services")
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

// ComposeServiceSpecs reads the resolved config JSON over the compose's
// manifest and env files: one ServiceSpec per service, sorted by name.
func ComposeServiceSpecs(ctx context.Context, compose *Compose, workdir string) ([]ServiceSpec, error) {
	out, err := compose.capture(ctx, workdir, "config", "--format", "json")
	if err != nil {
		return nil, err
	}
	return parseServiceSpecs(out)
}

// parseServiceSpecs decodes the `config --format json` document.
func parseServiceSpecs(raw []byte) ([]ServiceSpec, error) {
	var doc struct {
		Services map[string]struct {
			Restart     string `json:"restart"`
			Healthcheck *struct {
				Test    json.RawMessage `json:"test"`
				Disable bool            `json:"disable"`
			} `json:"healthcheck"`
		} `json:"services"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing docker compose config JSON: %w", err)
	}
	specs := make([]ServiceSpec, 0, len(doc.Services))
	for name, s := range doc.Services {
		hc := s.Healthcheck
		has := hc != nil && !hc.Disable && !healthcheckNone(hc.Test)
		specs = append(specs, ServiceSpec{Name: name, HasHealthcheck: has, Restart: s.Restart})
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	return specs, nil
}

// healthcheckNone: the test is ["NONE"], i.e. the healthcheck is disabled.
func healthcheckNone(raw json.RawMessage) bool {
	var t []string
	return json.Unmarshal(raw, &t) == nil && len(t) > 0 && t[0] == "NONE"
}

// capture runs `docker compose <manifest flags> <args...>` and returns stdout;
// a failure carries the last stderr line.
func (c *Compose) capture(ctx context.Context, workdir string, args ...string) ([]byte, error) {
	full := append(c.buildBaseArgs(), args...)
	cmd := exec.CommandContext(ctx, c.dockerBin(), full...)
	cmd.Dir = workdir
	var se bytes.Buffer
	cmd.Stderr = &se
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(lastLine(se.String())))
	}
	return out, nil
}

// ContainerState is one container as `compose ps -a` reports it.
type ContainerState struct {
	ID       string `json:"ID"`
	Name     string `json:"Name"`
	State    string `json:"State"`  // running, exited, restarting, ...
	Health   string `json:"Health"` // healthy, unhealthy, starting, or empty
	ExitCode int    `json:"ExitCode"`
}

// ComposeServiceStates returns every container (running or stopped) of service.
// Stopped containers matter: a one-shot is only visible once it has exited.
func (c *Compose) ComposeServiceStates(ctx context.Context, workdir, service string) ([]ContainerState, error) {
	out, err := c.capture(ctx, workdir, "ps", "-a", "--format", "json", service)
	if err != nil {
		return nil, err
	}
	return parseContainerStates(out)
}

// StateNotRestarted marks an exited container that has not started since the
// baseline (see FreshStates).
const StateNotRestarted = "not-restarted"

// startedAt returns the daemon's StartedAt string for container id.
func (c *Compose) startedAt(ctx context.Context, id string) (string, error) {
	out, err := exec.CommandContext(ctx, c.dockerBin(), "inspect", "-f", "{{.State.StartedAt}}", id).Output()
	if err != nil {
		return "", fmt.Errorf("docker inspect %s: %w", id, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// StartMarks maps container ID to StartedAt for each exited container of
// service. Taken before `up`, it lets FreshStates tell a one-shot that ran
// again (same container, new start time) from an old exit. Daemon strings are
// compared with each other only, so clock skew is moot.
func (c *Compose) StartMarks(ctx context.Context, workdir, service string) (map[string]string, error) {
	cs, err := c.ComposeServiceStates(ctx, workdir, service)
	if err != nil {
		return nil, err
	}
	marks := map[string]string{}
	for _, s := range cs {
		if s.State == "exited" && s.ID != "" {
			if marks[s.ID], err = c.startedAt(ctx, s.ID); err != nil {
				return nil, err
			}
		}
	}
	return marks, nil
}

// FreshStates is ComposeServiceStates, but an exited container whose StartedAt
// still equals its baseline mark is reported as StateNotRestarted, so a stale
// exit 0 cannot pass the one-shot gate. No ID (old compose) means no mark.
func (c *Compose) FreshStates(ctx context.Context, workdir, service string, baseline map[string]string) ([]ContainerState, error) {
	cs, err := c.ComposeServiceStates(ctx, workdir, service)
	if err != nil {
		return nil, err
	}
	for i, s := range cs {
		if old, ok := baseline[s.ID]; ok && s.State == "exited" {
			now, err := c.startedAt(ctx, s.ID)
			if err != nil {
				return nil, err
			}
			if now == old {
				cs[i].State = StateNotRestarted
			}
		}
	}
	return cs, nil
}

// parseContainerStates accepts both ps JSON shapes: one array (compose 2.21+)
// or one object per line (older).
func parseContainerStates(raw []byte) ([]ContainerState, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] != '[' {
		trimmed = append(append([]byte("["), bytes.ReplaceAll(trimmed, []byte("}\n"), []byte("},\n"))...), ']')
	}
	var states []ContainerState
	if err := json.Unmarshal(trimmed, &states); err != nil {
		return nil, fmt.Errorf("parsing compose ps JSON: %w", err)
	}
	return states, nil
}

// Gate failures: ErrOneShotFailed maps to E250, ErrGateTimeout to E251.
var (
	ErrOneShotFailed = errors.New("one-shot service exited non-zero")
	ErrGateTimeout   = errors.New("service did not reach its ready state in time")
)

// Clock is the time source of the gate. Sleep returns early with the context's
// error when ctx is cancelled.
type Clock interface {
	Now() time.Time
	Sleep(ctx context.Context, d time.Duration) error
}

// SystemClock is the real Clock.
type SystemClock struct{}

// Now returns the wall clock time.
func (SystemClock) Now() time.Time { return time.Now() }

// Sleep waits d or until ctx is done.
func (SystemClock) Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// StateSource reads the service's containers; a remote host supplies its own.
type StateSource func(ctx context.Context) ([]ContainerState, error)

// HealthGate waits until spec's containers are ready for their kind (D17):
// healthcheck declared -> every container healthy within HealthyTimeout
// (checked first, even with restart "no"); one-shot (restart "no", no
// healthcheck) -> every container exited 0 within HealthyTimeout, and any
// non-zero exit fails at once; otherwise -> running within
// RunningTimeout. A nil clock means SystemClock.
func HealthGate(ctx context.Context, spec ServiceSpec, states StateSource, clock Clock) error {
	if clock == nil {
		clock = SystemClock{}
	}
	limit := RunningTimeout
	if spec.HasHealthcheck || spec.OneShot() {
		limit = HealthyTimeout
	}
	deadline := clock.Now().Add(limit)
	for {
		cs, err := states(ctx)
		if err != nil {
			return fmt.Errorf("service %s: reading container state: %w", spec.Name, err)
		}
		ready, detail, failed := gateReady(spec, cs)
		if failed {
			return fmt.Errorf("service %s: %w (%s)", spec.Name, ErrOneShotFailed, detail)
		}
		if ready {
			return nil
		}
		if !clock.Now().Before(deadline) {
			return fmt.Errorf("service %s: %w within %s (%s)", spec.Name, ErrGateTimeout, limit, detail)
		}
		if err := clock.Sleep(ctx, gatePollEvery); err != nil {
			return fmt.Errorf("service %s: %w", spec.Name, err)
		}
	}
}

// gateReady evaluates one poll. failed is true only for a one-shot that
// exited non-zero (no point waiting on it).
func gateReady(spec ServiceSpec, cs []ContainerState) (ready bool, detail string, failed bool) {
	if len(cs) == 0 {
		return false, "no container found", false
	}
	ready = true
	for _, c := range cs {
		wait := ""
		switch {
		case spec.OneShot():
			if c.State == "exited" && c.ExitCode != 0 {
				return false, fmt.Sprintf("%s exited %d", c.Name, c.ExitCode), true
			}
			if c.State != "exited" {
				wait = "exit 0"
			}
		case spec.HasHealthcheck:
			if c.State != "running" || c.Health != "healthy" {
				wait = "healthy"
			}
		case c.State != "running":
			wait = "running"
		}
		if wait != "" {
			ready, detail = false, fmt.Sprintf("%s is %s/%s, waiting for %s", c.Name, c.State, c.Health, wait)
		}
	}
	return ready, detail, false
}
