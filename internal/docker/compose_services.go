package docker

// compose_services.go — resolved per-service facts and the per-service health
// gate used by the rolling deploy (D17, D-0041).
//
// Purpose: read what the resolved compose model says about each service (does
// it declare a healthcheck, is it a one-shot with restart "no") and wait for a
// restarted service to reach the state its kind promises. The manifest compose
// (every -f file plus every --env-file) is the only input, so plugin fragments
// and their computed variables are part of the answer, exactly as for restart.
// Inputs: a *Compose carrying the manifest files and env files, a workdir.
// Outputs: ServiceSpec values, container states, and a typed gate error.
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

// Gate timeouts by service kind (D17).
const (
	HealthyTimeout = 60 * time.Second // healthcheck declared, or one-shot exit
	RunningTimeout = 30 * time.Second // no healthcheck
	gatePollEvery  = 2 * time.Second
)

// ServiceSpec is what the resolved compose model says about one service.
type ServiceSpec struct {
	Name           string `json:"name"`
	HasHealthcheck bool   `json:"has_healthcheck"`
	Restart        string `json:"restart"`
}

// OneShot reports a service that is meant to run once and exit (restart "no").
func (s ServiceSpec) OneShot() bool { return s.Restart == "no" }

// ComposeServiceNames returns `docker compose config --services` over the
// compose's manifest files and env files, in the order docker prints them.
func (c *Compose) ComposeServiceNames(ctx context.Context, workdir string) ([]string, error) {
	out, err := c.capture(ctx, workdir, "config", "--services")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			names = append(names, s)
		}
	}
	return names, nil
}

// ComposeServiceSpecs reads the resolved config JSON over the compose's
// manifest and env files and returns one ServiceSpec per service, sorted by
// name.
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
		has := false
		if hc := s.Healthcheck; hc != nil && !hc.Disable {
			has = !strings.Contains(string(hc.Test), `"NONE"`)
		}
		specs = append(specs, ServiceSpec{Name: name, HasHealthcheck: has, Restart: s.Restart})
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	return specs, nil
}

// capture runs `docker compose <manifest flags> <args...>` and returns stdout.
// A failure carries the last stderr text so "no such service" stays visible.
func (c *Compose) capture(ctx context.Context, workdir string, args ...string) ([]byte, error) {
	full := append(c.buildBaseArgs(), args...)
	cmd := exec.CommandContext(ctx, c.dockerBin(), full...)
	cmd.Dir = workdir
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(lastLine(se.String())))
	}
	return so.Bytes(), nil
}

// ContainerState is one container of a service as `compose ps -a` reports it.
type ContainerState struct {
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

// parseContainerStates accepts both ps JSON shapes: one array (compose 2.21+)
// or one object per line (older).
func parseContainerStates(raw []byte) ([]ContainerState, error) {
	var states []ContainerState
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &states); err != nil {
			return nil, fmt.Errorf("parsing compose ps JSON array: %w", err)
		}
	} else {
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		for dec.More() {
			var c ContainerState
			if err := dec.Decode(&c); err != nil {
				return nil, fmt.Errorf("parsing compose ps JSON: %w", err)
			}
			states = append(states, c)
		}
	}
	return states, nil
}

// Gate failures. Callers map them to codes: ErrOneShotFailed is E250 (the
// service is broken), ErrGateTimeout is E251 (it never got there in time).
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

// StateSource reads the current containers of the service under test. It is a
// function so a remote host can supply its own reader.
type StateSource func(ctx context.Context) ([]ContainerState, error)

// HealthGate waits until spec's containers are ready for their kind (D17):
// healthcheck declared -> every container healthy within HealthyTimeout;
// one-shot (restart "no") -> every container exited 0 within HealthyTimeout,
// and any non-zero exit fails at once; otherwise -> running within
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
	var last string
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
		last = detail
		if !clock.Now().Before(deadline) {
			return fmt.Errorf("service %s: %w within %s (%s)", spec.Name, ErrGateTimeout, limit, last)
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
		switch {
		case spec.OneShot():
			if c.State == "exited" && c.ExitCode != 0 {
				return false, fmt.Sprintf("%s exited %d", c.Name, c.ExitCode), true
			}
			if c.State != "exited" {
				ready, detail = false, fmt.Sprintf("%s is %s, waiting for exit 0", c.Name, c.State)
			}
		case spec.HasHealthcheck:
			if c.State != "running" || c.Health != "healthy" {
				ready, detail = false, fmt.Sprintf("%s is %s/%s, waiting for healthy", c.Name, c.State, c.Health)
			}
		default:
			if c.State != "running" {
				ready, detail = false, fmt.Sprintf("%s is %s, waiting for running", c.Name, c.State)
			}
		}
	}
	return ready, detail, false
}
