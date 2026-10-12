package docker

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// initContainerListArgs is the `docker ps` argument list that selects init
// containers safe to remove: labelled nself.type=init-container, exited, with
// exit code 0. Running ones (a start in progress, or a service that uses the
// label) and failed ones (kept for `docker logs`) are left alone.
func initContainerListArgs() []string {
	return []string{"ps", "-aq",
		"--filter", "label=nself.type=init-container",
		"--filter", "status=exited",
		"--filter", "exited=0",
	}
}

// cleanupInitContainers removes finished init containers (minio-init,
// meilisearch-init, seaweedfs-init, ...): the ones that exited with code 0.
// Running and failed init containers are kept.
func cleanupInitContainers(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, "docker", initContainerListArgs()...).Output()
	if err != nil {
		return fmt.Errorf("finding init containers: %w", err)
	}
	for _, id := range parseLines(string(out)) {
		if err := forceRemoveContainer(ctx, id); err != nil {
			// Best-effort: log but don't fail on individual removals.
			continue
		}
	}
	return nil
}

// CleanupZombieContainers finds containers in "created" state belonging to the
// given compose project and removes them. These are zombie containers left from
// interrupted starts.
func cleanupZombieContainers(ctx context.Context, projectName string) error {
	// Find containers belonging to this project that are in "created" state.
	filter := fmt.Sprintf("com.docker.compose.project=%s", projectName)
	ids, err := findContainersByLabelAndStatus(ctx, filter, "created")
	if err != nil {
		return fmt.Errorf("finding zombie containers: %w", err)
	}
	for _, id := range ids {
		if err := forceRemoveContainer(ctx, id); err != nil {
			continue
		}
	}
	return nil
}

// renamedLeftoverRE matches Docker Compose's rename-during-recreate name:
// when a service with a fixed container_name is recreated, Compose renames
// the old container to "<hex-id>_<original-name>" before creating the new
// one. If the recreate fails or is interrupted, the hash-prefixed container
// survives (e.g. b6d7b59a1c78_ntask_hasura) and shadows the clean
// <app>_<service> name that Makefiles and docs rely on (ntask gap #21).
var renamedLeftoverRE = regexp.MustCompile(`^[0-9a-f]{8,}_`)

// isRenamedComposeLeftover reports whether a container name is a stale
// Compose rename-leftover for the given project: a hex-id prefix followed by
// a name that belongs to the project ("<hex>_<project>_...").
func isRenamedComposeLeftover(name, projectName string) bool {
	m := renamedLeftoverRE.FindString(name)
	if m == "" {
		return false
	}
	rest := name[len(m):]
	return strings.HasPrefix(rest, projectName+"_")
}

// cleanupRenamedLeftovers removes stale hash-prefixed rename-leftover
// containers for the project so the clean <project>_<service> container
// names are always the ones running. Best-effort: individual failures are
// skipped.
func cleanupRenamedLeftovers(ctx context.Context, projectName string) error {
	cmd := exec.CommandContext(ctx, "docker", "ps", "-a", "--format", "{{.Names}}")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("docker ps -a: %w", err)
	}
	for _, name := range parseLines(string(out)) {
		if !isRenamedComposeLeftover(name, projectName) {
			continue
		}
		slog.Info("removing stale renamed container leftover", "container", name)
		if err := forceRemoveContainer(ctx, name); err != nil {
			continue
		}
	}
	return nil
}

// RunPreStartCleanup removes stale artifacts that would break `docker compose
// up`: hash-prefixed rename-leftover containers (which hold ports and shadow
// clean container names). Run before compose up.
func RunPreStartCleanup(ctx context.Context, projectName string) error {
	return cleanupRenamedLeftovers(ctx, projectName)
}

// RunPostStartCleanup runs the full post-startup cleanup sequence:
//  1. Init container cleanup, repeated 3 times with 500ms delays (handles race conditions
//     where containers are still being marked as exited).
//  2. Zombie container cleanup for containers stuck in "created" state.
func RunPostStartCleanup(ctx context.Context, projectName string) error {
	// Run init container cleanup 3 times with delays to handle race conditions.
	for i := 0; i < 3; i++ {
		if err := cleanupInitContainers(ctx); err != nil {
			// Non-fatal: continue with remaining passes.
			slog.Debug("cleanup init containers (non-fatal)", "pass", i+1, "err", err)
		}
		if i < 2 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
	}

	// Clean up zombie containers from interrupted starts.
	if err := cleanupZombieContainers(ctx, projectName); err != nil {
		// Non-fatal: best effort.
		slog.Debug("cleanup zombie containers (non-fatal)", "project", projectName, "err", err)
	}

	// Clean up hash-prefixed rename-leftovers from interrupted recreates so
	// the clean <project>_<service> names are the surviving containers.
	if err := cleanupRenamedLeftovers(ctx, projectName); err != nil {
		// Non-fatal: best effort.
		slog.Debug("cleanup renamed leftovers (non-fatal)", "project", projectName, "err", err)
	}

	return nil
}

// findContainersByLabel runs `docker ps -aq --filter label=<label>` and returns
// the container IDs (including stopped containers).
func findContainersByLabel(ctx context.Context, label string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "docker", "ps", "-aq", "--filter", fmt.Sprintf("label=%s", label))
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker ps filter label=%s: %w", label, err)
	}
	return parseLines(string(out)), nil
}

// findContainersByLabelAndStatus runs `docker ps -aq` filtering by both a label
// and a container status (e.g., "created", "exited").
func findContainersByLabelAndStatus(ctx context.Context, label, status string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "docker", "ps", "-aq",
		"--filter", fmt.Sprintf("label=%s", label),
		"--filter", fmt.Sprintf("status=%s", status),
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker ps filter label=%s status=%s: %w", label, status, err)
	}
	return parseLines(string(out)), nil
}

// forceRemoveContainer runs `docker rm -f <id>`.
func forceRemoveContainer(ctx context.Context, id string) error {
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", id)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker rm -f %s: %w (%s)", id, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// parseLines splits command output into non-empty trimmed lines.
func parseLines(output string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
