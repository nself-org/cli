package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// FileBindMounted reports whether any container binds this exact host file.
// Stopped containers count too. Unavailable or incomplete inspection is an error,
// so callers can preserve the inode whenever its mount status is uncertain.
func FileBindMounted(ctx context.Context, path string) (bool, error) {
	ids, err := exec.CommandContext(ctx, "docker", "ps", "-aq").Output()
	if err != nil {
		return false, err
	}
	names := strings.Fields(string(ids))
	if len(names) == 0 {
		return false, nil
	}
	data, err := exec.CommandContext(ctx, "docker", append([]string{"inspect"}, names...)...).Output()
	if err != nil {
		return false, err
	}
	var containers []struct{ Mounts []struct{ Source string } }
	if err := json.Unmarshal(data, &containers); err != nil {
		return false, err
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false, fmt.Errorf("cannot resolve bind mount path: %w", err)
	}
	for _, container := range containers {
		for _, mount := range container.Mounts {
			source, err := filepath.EvalSymlinks(mount.Source)
			if err != nil {
				return false, fmt.Errorf("cannot resolve bind mount path: %w", err)
			}
			if source == target {
				return true, nil
			}
		}
	}
	return false, nil
}
