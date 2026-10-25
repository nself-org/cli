package docker

// compose_states.go — container state reads for the per-service health gate.
//
// Purpose: report what `docker compose ps -a` says about the containers of one
// service, without the one-off containers `docker compose run` leaves behind,
// and tell a fresh run from a stale exit using the daemon's StartedAt.
// Constraints: every docker call goes through this package (the docker
// funnel); StartedAt strings are only compared with each other (no clock skew).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Bounds on one docker read, so a hung daemon cannot hang a deploy. They are
// vars only so tests can shorten them. stateWaitDelay is how long a killed
// docker CLI may keep its pipes open: `docker compose` runs the compose plugin
// as a child, and killing the CLI leaves that child holding stdout.
var (
	stateCallTimeout = 30 * time.Second
	stateWaitDelay   = 2 * time.Second
)

// ErrStateTimeout is wrapped by a state read whose own bound fired. It is not
// context.DeadlineExceeded on purpose: callers outside the gate class it as a
// state-read failure, not as the user's interrupt.
var ErrStateTimeout = errors.New("docker gave no answer in time")

// oneOffLabel is set to "True" by Compose on containers made by `compose run`.
const oneOffLabel = "com.docker.compose.oneoff"

// ContainerState is one container as `compose ps -a` reports it.
type ContainerState struct {
	ID        string `json:"ID"`
	Name      string `json:"Name"`
	State     string `json:"State"`  // running, exited, restarting, ...
	Health    string `json:"Health"` // healthy, unhealthy, starting, or empty
	ExitCode  int    `json:"ExitCode"`
	StartedAt string `json:"-"` // daemon StartedAt; set by FreshStates, empty from a plain read

	// Labels is a map or one "k=v,k=v" string, depending on the compose release.
	Labels json.RawMessage `json:"Labels,omitempty"`
}

// OneOff reports a container created by `docker compose run`. It is not a
// replica of the service, so the gate must not wait on it.
func (s ContainerState) OneOff() bool {
	var m map[string]string
	if json.Unmarshal(s.Labels, &m) == nil {
		return strings.EqualFold(m[oneOffLabel], "true")
	}
	var csv string
	if json.Unmarshal(s.Labels, &csv) != nil {
		return false
	}
	for _, kv := range strings.Split(csv, ",") {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.TrimSpace(k) == oneOffLabel {
			return strings.EqualFold(strings.TrimSpace(v), "true")
		}
	}
	return false
}

// ComposeServiceStates returns every replica (running or stopped) of service.
// Stopped containers matter: a one-shot is only visible once it has exited.
// One-off `compose run` containers are dropped. The read is bounded by
// stateCallTimeout.
func (c *Compose) ComposeServiceStates(ctx context.Context, workdir, service string) ([]ContainerState, error) {
	var cs []ContainerState
	err := bounded(ctx, "docker compose ps for "+service, func(ctx context.Context) (err error) {
		cs, err = c.serviceStates(ctx, workdir, service)
		return err
	})
	return cs, err
}

// serviceStates is ComposeServiceStates without its own bound.
func (c *Compose) serviceStates(ctx context.Context, workdir, service string) ([]ContainerState, error) {
	out, err := c.capture(ctx, workdir, "ps", "-a", "--format", "json", "--", service)
	if err != nil {
		return nil, err
	}
	all, err := ParseContainerStates(out)
	if err != nil {
		return nil, err
	}
	kept := all[:0]
	for _, s := range all {
		if !s.OneOff() {
			kept = append(kept, s)
		}
	}
	return kept, nil
}

// StateNotRestarted marks an exited container that has not started since the
// baseline (see FreshStates).
const StateNotRestarted = "not-restarted"

// errContainerGone: docker inspect found no such container (it was removed
// between the ps and the inspect).
var errContainerGone = errors.New("container is gone")

// bounded runs fn under one stateCallTimeout covering the whole read, and
// classifies a failure with stateCallErr.
func bounded(parent context.Context, what string, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(parent, stateCallTimeout)
	defer cancel()
	if err := fn(ctx); err != nil {
		return stateCallErr(parent, ctx, what, err)
	}
	return nil
}

// stateCallErr classifies a failed docker read. A done parent ctx wins: the
// error wraps the parent's error (so a cancel reads as a cancel) and err. The
// read's own deadline wraps ErrStateTimeout, never context.DeadlineExceeded,
// so callers do not mistake a hung daemon for the user's interrupt.
func stateCallErr(parent, call context.Context, what string, err error) error {
	if perr := parent.Err(); perr != nil {
		return fmt.Errorf("%w: %w", perr, err)
	}
	if errors.Is(call.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%s gave no answer within %s: %w", what, stateCallTimeout, ErrStateTimeout)
	}
	return err
}

// boundedOutput runs cmd with a soft cancel and a WaitDelay (see softCancel and
// stateWaitDelay). A clean exit whose pipes a grandchild still held is a
// success: the output is complete.
func boundedOutput(cmd *exec.Cmd) ([]byte, error) {
	softCancel(cmd)
	cmd.WaitDelay = stateWaitDelay
	out, err := cmd.Output()
	if errors.Is(err, exec.ErrWaitDelay) {
		err = nil
	}
	return out, err
}

// startedAt returns the daemon's StartedAt string for container id. The caller
// bounds it.
func (c *Compose) startedAt(ctx context.Context, id string) (string, error) {
	cmd := exec.CommandContext(ctx, c.dockerBin(), "inspect", "-f", "{{.State.StartedAt}}", "--", id)
	var se bytes.Buffer
	cmd.Stderr = &se
	out, err := boundedOutput(cmd)
	if err != nil {
		if low := strings.ToLower(se.String()); strings.Contains(low, "no such object") || strings.Contains(low, "no such container") {
			err = errContainerGone
		}
		return "", fmt.Errorf("docker inspect %s: %w", id, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// StartMarks maps container ID to StartedAt for each exited container of
// service. Taken before `up`, it lets FreshStates tell a one-shot that ran
// again (same container, new start time) from an old exit. The whole read
// (ps and every inspect) shares one stateCallTimeout.
func (c *Compose) StartMarks(ctx context.Context, workdir, service string) (map[string]string, error) {
	marks := map[string]string{}
	err := bounded(ctx, "docker state read for "+service, func(ctx context.Context) error {
		cs, err := c.serviceStates(ctx, workdir, service)
		if err != nil {
			return err
		}
		for _, s := range cs {
			if s.State == "exited" && s.ID != "" {
				if marks[s.ID], err = c.startedAt(ctx, s.ID); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return marks, nil
}

// FreshStates is ComposeServiceStates plus StartedAt on every running or
// exited container. An exited container whose StartedAt still equals its
// baseline mark is reported as StateNotRestarted, so a stale exit cannot pass
// the one-shot gate. No ID (old compose) means no mark and no StartedAt. A
// container removed between the ps and its inspect is dropped from the result
// (the gate polls again; Regressed reports it gone). The whole read shares one
// stateCallTimeout.
func (c *Compose) FreshStates(ctx context.Context, workdir, service string, baseline map[string]string) ([]ContainerState, error) {
	var kept []ContainerState
	err := bounded(ctx, "docker state read for "+service, func(ctx context.Context) error {
		cs, err := c.serviceStates(ctx, workdir, service)
		if err != nil {
			return err
		}
		kept = cs[:0]
		for _, s := range cs {
			if s.ID != "" && (s.State == "running" || s.State == "exited") {
				now, err := c.startedAt(ctx, s.ID)
				if errors.Is(err, errContainerGone) {
					continue
				}
				if err != nil {
					return err
				}
				s.StartedAt = now
				if old, ok := baseline[s.ID]; ok && s.State == "exited" && now == old {
					s.State = StateNotRestarted
				}
			}
			kept = append(kept, s)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return kept, nil
}

// ParseContainerStates accepts both ps JSON shapes: one array or one object per
// line, depending on the compose release; blank lines and CRLF are tolerated.
// internal/deploy calls it on the output it reads over ssh.
func ParseContainerStates(raw []byte) ([]ContainerState, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}
	var states []ContainerState
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &states); err != nil {
			return nil, fmt.Errorf("parsing compose ps JSON: %w", err)
		}
		return states, nil
	}
	for _, line := range bytes.Split(trimmed, []byte("\n")) {
		if line = bytes.TrimSpace(line); len(line) == 0 {
			continue
		}
		var s ContainerState
		if err := json.Unmarshal(line, &s); err != nil {
			return nil, fmt.Errorf("parsing compose ps JSON: %w", err)
		}
		states = append(states, s)
	}
	return states, nil
}
