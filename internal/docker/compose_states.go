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
	"fmt"
	"os/exec"
	"strings"
)

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

	// Labels is a map (Compose 2.21+) or one "k=v,k=v" string (older).
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
// One-off `compose run` containers are dropped.
func (c *Compose) ComposeServiceStates(ctx context.Context, workdir, service string) ([]ContainerState, error) {
	out, err := c.capture(ctx, workdir, "ps", "-a", "--format", "json", service)
	if err != nil {
		return nil, err
	}
	all, err := parseContainerStates(out)
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
// again (same container, new start time) from an old exit.
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

// FreshStates is ComposeServiceStates plus StartedAt on every running or
// exited container. An exited container whose StartedAt still equals its
// baseline mark is reported as StateNotRestarted, so a stale exit cannot pass
// the one-shot gate. No ID (old compose) means no mark and no StartedAt.
func (c *Compose) FreshStates(ctx context.Context, workdir, service string, baseline map[string]string) ([]ContainerState, error) {
	cs, err := c.ComposeServiceStates(ctx, workdir, service)
	if err != nil {
		return nil, err
	}
	for i, s := range cs {
		if s.ID == "" || (s.State != "running" && s.State != "exited") {
			continue
		}
		now, err := c.startedAt(ctx, s.ID)
		if err != nil {
			return nil, err
		}
		cs[i].StartedAt = now
		if old, ok := baseline[s.ID]; ok && s.State == "exited" && now == old {
			cs[i].State = StateNotRestarted
		}
	}
	return cs, nil
}

// parseContainerStates accepts both ps JSON shapes: one array (compose 2.21+)
// or one object per line (older); blank lines and CRLF are tolerated.
func parseContainerStates(raw []byte) ([]ContainerState, error) {
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

// Regressed compares the containers a service's gate saw (before) with a later
// read (after). It returns ErrServiceCrashed, with the container named, for one
// that is restarting, exited non-zero, or started again since the gate passed
// (a new StartedAt). Rows without an ID or StartedAt cannot be compared by start
// time and are only judged on state.
func Regressed(before, after []ContainerState) error {
	started := map[string]string{}
	for _, b := range before {
		if b.State == "running" && b.ID != "" && b.StartedAt != "" {
			started[b.ID] = b.StartedAt
		}
	}
	for _, a := range after {
		switch {
		case a.State == "restarting":
			return fmt.Errorf("%w (%s is restarting after its gate passed)", ErrServiceCrashed, a.Name)
		case a.State == "exited" && a.ExitCode != 0:
			return fmt.Errorf("%w (%s exited %d after its gate passed)", ErrServiceCrashed, a.Name, a.ExitCode)
		}
		if was, ok := started[a.ID]; ok && a.StartedAt != "" && a.StartedAt != was {
			return fmt.Errorf("%w (%s restarted after its gate passed)", ErrServiceCrashed, a.Name)
		}
	}
	return nil
}
