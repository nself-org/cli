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
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Gate timeouts by service kind (D17): healthcheck or one-shot exit, else running
// (and still the same run one poll later).
const (
	HealthyTimeout = 60 * time.Second
	RunningTimeout = 30 * time.Second
	gatePollEvery  = 2 * time.Second
)

// gateReadGrace is the least time one gate state read may take, even past the
// gate's deadline: the one read after the deadline still counts, but a hung
// read is cut off. A var so tests can shorten it.
var gateReadGrace = 10 * time.Second

// ServiceSpec is what the resolved compose model says about one service.
type ServiceSpec struct {
	Name           string `json:"name"`
	HasHealthcheck bool   `json:"has_healthcheck"`
	Restart        string `json:"restart"`

	// HealthTimeout is how long Docker may take to declare the healthcheck
	// failed: start_period + retries*(interval+timeout). 0 without a healthcheck
	// or when unknown (a spec built elsewhere). The latest Docker can still
	// report healthy before it would declare the container unhealthy.
	HealthTimeout time.Duration `json:"-"`
}

// OneShot reports a service that is meant to run once and exit: restart "no"
// and no healthcheck. A declared healthcheck wins (D17 order): such a service
// is long-running and is gated on healthy, never on exit.
func (s ServiceSpec) OneShot() bool { return s.Restart == "no" && !s.HasHealthcheck }

// ComposeServiceNames returns `docker compose config --services` over the
// manifest and env files, in docker's order.
func (c *Compose) ComposeServiceNames(ctx context.Context, workdir string) ([]string, error) {
	out, err := c.captureBounded(ctx, "docker compose config", workdir, "config", "--services")
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

// capture runs `docker compose <manifest flags> <args...>` and returns stdout;
// a failure carries the last stderr line.
func (c *Compose) capture(ctx context.Context, workdir string, args ...string) ([]byte, error) {
	full := append(c.buildBaseArgs(), args...)
	cmd := exec.CommandContext(ctx, c.dockerBin(), full...)
	cmd.Dir = workdir
	var se bytes.Buffer
	cmd.Stderr = &se
	out, err := boundedOutput(cmd)
	if err != nil {
		return nil, fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(lastLine(se.String())))
	}
	return out, nil
}

// captureBounded is capture under one stateCallTimeout.
func (c *Compose) captureBounded(ctx context.Context, what, workdir string, args ...string) ([]byte, error) {
	var out []byte
	err := bounded(ctx, what, func(ctx context.Context) (err error) {
		out, err = c.capture(ctx, workdir, args...)
		return err
	})
	return out, err
}

// Gate failures. ErrOneShotFailed and ErrServiceCrashed map to E250 (the
// service is broken), ErrGateTimeout to E251 (too slow), ErrStateRead and
// ErrStateTimeout to no service code at all: docker could not answer, which
// says nothing about the service.
var (
	ErrOneShotFailed  = errors.New("one-shot service exited non-zero")
	ErrServiceCrashed = errors.New("service exited non-zero or is restarting")
	ErrGateTimeout    = errors.New("service did not reach its ready state in time")
	ErrStateRead      = errors.New("could not read container state")
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
// StartedAt on a running container lets the gate see a restart between polls;
// a source that leaves it empty still gets the two-poll check.
type StateSource func(ctx context.Context) ([]ContainerState, error)

// HealthGate waits until spec's containers are ready for their kind (D17).
// Any kind fails at once on a container that exited non-zero or is
// restarting. Healthcheck declared -> every container healthy within
// HealthyTimeout (checked first, even with restart "no"). One-shot (restart
// "no", no healthcheck) -> every container exited 0 within HealthyTimeout.
// Otherwise -> every container running within RunningTimeout and seen running
// on two polls in a row with the same StartedAt (a crash loop passes one
// poll); a service with no restart line that exited 0 is done, as Docker
// treats it. A healthcheck service waits max(HealthyTimeout, spec.HealthTimeout).
// Each state read is bounded (see gateReadGrace); a read that gets no answer
// wraps ErrGateTimeout. A cancelled ctx returns ctx's error; a failed state read wraps
// ErrStateRead. A nil clock means SystemClock.
func HealthGate(ctx context.Context, spec ServiceSpec, states StateSource, clock Clock) error {
	if clock == nil {
		clock = SystemClock{}
	}
	limit := RunningTimeout
	if spec.HasHealthcheck || spec.OneShot() {
		limit = HealthyTimeout
	}
	if spec.HasHealthcheck {
		limit = max(limit, spec.HealthTimeout)
	}
	deadline := clock.Now().Add(limit)
	var prev map[string]string
	for {
		budget := max(deadline.Sub(clock.Now()), gateReadGrace)
		readCtx, cancel := context.WithTimeout(ctx, budget)
		cs, err := states(readCtx)
		cut := readCtx.Err() != nil // before cancel(), which sets it too
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("service %s: %w", spec.Name, ctx.Err())
			}
			if cut || errors.Is(err, ErrStateTimeout) {
				return fmt.Errorf("service %s: %w (container state read gave no answer within %s)", spec.Name, ErrGateTimeout, budget)
			}
			return fmt.Errorf("service %s: %w: %w", spec.Name, ErrStateRead, err)
		}
		if ctx.Err() != nil { // cancelled while the read was in flight
			return fmt.Errorf("service %s: %w", spec.Name, ctx.Err())
		}
		ready, detail, next, fail := gateReady(spec, cs, prev)
		if fail != nil {
			return fmt.Errorf("service %s: %w (%s)", spec.Name, fail, detail)
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
		prev = next
	}
}

// gateReady evaluates one poll against prev, the StartedAt of each container
// seen running on the poll before. fail is non-nil for a container that will
// not recover on its own (no point waiting). next is this poll's running set.
func gateReady(spec ServiceSpec, cs []ContainerState, prev map[string]string) (ready bool, detail string, next map[string]string, fail error) {
	next = map[string]string{}
	if len(cs) == 0 {
		return false, "no container found", next, nil
	}
	ready = true
	for _, c := range cs {
		key := c.ID
		if key == "" {
			key = c.Name
		}
		if c.State == "running" {
			next[key] = c.StartedAt
		}
		if c.State == "restarting" || c.State == "dead" || (c.State == "exited" && c.ExitCode != 0) {
			fail = ErrServiceCrashed
			if spec.OneShot() {
				fail = ErrOneShotFailed
			}
			if c.State == "restarting" || c.State == "dead" {
				return false, c.Name + " is " + c.State, next, fail
			}
			return false, fmt.Sprintf("%s exited %d", c.Name, c.ExitCode), next, fail
		}
		wait := ""
		switch {
		case spec.OneShot():
			if c.State != "exited" {
				wait = "exit 0"
			}
		case spec.HasHealthcheck:
			if c.State != "running" || c.Health != "healthy" {
				wait = "healthy"
			}
		case c.State == "exited" && spec.Restart == "":
			// exit 0 and Docker's default restart policy ("no"): a finished run.
		case c.State != "running":
			wait = "running"
		default:
			if at, seen := prev[key]; !seen || at != c.StartedAt {
				wait = "the same run on a second poll"
			}
		}
		if wait != "" {
			ready, detail = false, fmt.Sprintf("%s is %s/%s, waiting for %s", c.Name, c.State, c.Health, wait)
		}
	}
	return ready, detail, next, nil
}
