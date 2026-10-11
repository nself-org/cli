package deploy

// Purpose: container state reads on a remote host for the P7-DEPL-16 health
//          gate (docker.HealthGate / docker.Regressed), so a remote apply is
//          gated exactly like the local rolling restart.
// Inputs:  a Remote, the remote project path, the shipped manifests, a service.
// Outputs: docker.ContainerState rows (with StartedAt) and the start marks
//          taken before `up`.
// Constraints: `docker compose ps -a --format json` and `docker inspect` over
//              ssh. The rows are decoded into the exported docker.ContainerState
//              and filtered with its OneOff(); internal/docker exports no
//              decoder for the ps document (both shapes, array and one object
//              per line, are handled here). Same StartedAt rules as
//              docker.Compose.FreshStates.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/docker"
)

// RemoteStates reads service's containers on the host. baseline (from
// RemoteStartMarks) turns an exited container that has not started since into
// docker.StateNotRestarted; nil means no baseline.
func RemoteStates(ctx context.Context, r Remote, remotePath string, files, envFiles []string, service string, baseline map[string]string) ([]docker.ContainerState, error) {
	cmd := fmt.Sprintf("cd %s && %s ps -a --format json %s", q(remotePath), composeBase(files, envFiles), q(service))
	out, err := r.Run(ctx, shScript(cmd))
	if err != nil {
		return nil, err
	}
	all, err := decodeContainerStates(out)
	if err != nil {
		return nil, err
	}
	var kept []docker.ContainerState
	var ids []string
	idx := map[string]int{}
	for _, s := range all {
		if s.OneOff() {
			continue
		}
		kept = append(kept, s)
		if s.ID != "" && (s.State == "running" || s.State == "exited") {
			idx[s.ID] = len(kept) - 1
			ids = append(ids, s.ID)
		}
	}
	started, err := startedAt(ctx, r, ids)
	if err != nil {
		return nil, err
	}
	for i, id := range ids {
		c := &kept[idx[id]]
		c.StartedAt = started[i]
		if old, ok := baseline[id]; ok && c.State == "exited" && c.StartedAt == old {
			c.State = docker.StateNotRestarted
		}
	}
	return kept, nil
}

// RemoteStartMarks maps the id of each exited container of service to its
// StartedAt, taken before `up` so a one-shot that runs again can be told from
// an old exit.
func RemoteStartMarks(ctx context.Context, r Remote, remotePath string, files, envFiles []string, service string) (map[string]string, error) {
	cs, err := RemoteStates(ctx, r, remotePath, files, envFiles, service, nil)
	if err != nil {
		return nil, err
	}
	marks := map[string]string{}
	for _, c := range cs {
		if c.State == "exited" && c.ID != "" {
			marks[c.ID] = c.StartedAt
		}
	}
	return marks, nil
}

// startedAt returns the daemon StartedAt of each id, in order.
func startedAt(ctx context.Context, r Remote, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out, err := r.Run(ctx, shScript("docker inspect -f '{{.State.StartedAt}}' "+strings.Join(quoteAll(ids), " ")))
	if err != nil {
		return nil, fmt.Errorf("docker inspect: %w", err)
	}
	lines := strings.Fields(string(out))
	if len(lines) != len(ids) {
		return nil, fmt.Errorf("docker inspect returned %d start times for %d containers", len(lines), len(ids))
	}
	return lines, nil
}

// decodeContainerStates accepts one JSON array or one object per line.
func decodeContainerStates(raw []byte) ([]docker.ContainerState, error) {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 {
		return nil, nil
	}
	var rows []docker.ContainerState
	if t[0] == '[' {
		if err := json.Unmarshal(t, &rows); err != nil {
			return nil, fmt.Errorf("parsing compose ps JSON: %w", err)
		}
		return rows, nil
	}
	for _, line := range bytes.Split(t, []byte("\n")) {
		if line = bytes.TrimSpace(line); len(line) == 0 {
			continue
		}
		var s docker.ContainerState
		if err := json.Unmarshal(line, &s); err != nil {
			return nil, fmt.Errorf("parsing compose ps JSON: %w", err)
		}
		rows = append(rows, s)
	}
	return rows, nil
}
