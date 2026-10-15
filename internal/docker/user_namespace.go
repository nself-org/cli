package docker

// Purpose: detect daemon user-namespace remapping before choosing bind-mount IDs.
// Inputs: a context bounding the Docker info call against the selected daemon.
// Outputs: whether rootless or userns remapping is enabled, or a probe error.
// Constraints: read-only; callers decide how offline builds handle probe errors.
import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
)

// UserNamespaceRemapped reports whether container IDs differ from host IDs.
func UserNamespaceRemapped(ctx context.Context) (bool, error) {
	out, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{json .SecurityOptions}}").Output()
	if err != nil {
		return false, err
	}
	var options []string
	if err := json.Unmarshal(out, &options); err != nil {
		return false, err
	}
	for _, option := range options {
		name, _, _ := strings.Cut(option, ",")
		if name == "name=rootless" || name == "name=userns" {
			return true, nil
		}
	}
	return false, nil
}
