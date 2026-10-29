package postgres

// image_warning.go — warns when a build-time postgres image would
// change what a currently running postgres container is using.
//
// Purpose: cli#384 — `nself build` must never silently regenerate a compose
// file that swaps a populated, running postgres/pgvector container for a
// different image. Restarting postgres under a swapped image is a data-loss
// event (P1 EOP staging incident 2026-06-10). This surfaces the mismatch as a
// build-time WARNING so the operator can pin POSTGRES_IMAGE before applying
// the regenerated compose file with `nself start`.
// Inputs: containerName — the postgres container name (<project>_postgres);
//         generatedImage — the image `nself build` is about to write.
// Outputs: a non-empty warning string when the running and generated images
//          differ; "" when they match, the container isn't running, or
//          Docker isn't reachable.
// Constraints: best-effort and advisory only — never returns an error, never
//              blocks or modifies the build, never shells out for more than
//              a few seconds.

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// runningContainerImageTimeout bounds the `docker inspect` call so a hung or
// unreachable Docker daemon cannot stall `nself build`.
const runningContainerImageTimeout = 3 * time.Second

// runningContainerImage returns the image:tag reference (Config.Image, not
// the sha256 image ID) that containerName was created from. Returns "" when
// Docker isn't reachable or the container doesn't exist — callers must treat
// that as "nothing to compare", never as an error.
func runningContainerImage(containerName string) string {
	ctx, cancel := context.WithTimeout(context.Background(), runningContainerImageTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.Config.Image}}", containerName).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ImageChangeWarning returns a WARNING message when the postgres
// image `nself build` is about to generate differs from the image the
// currently running postgres container was started from. Returns "" when
// there is nothing to warn about (no running container, or the images match).
func ImageChangeWarning(containerName, generatedImage string) string {
	return imageChangeWarning(containerName, generatedImage, runningContainerImage)
}

// imageChangeWarning is the testable core of
// ImageChangeWarning, parameterized by an image-lookup function so
// tests can inject a fake baseline without a reachable Docker daemon.
func imageChangeWarning(containerName, generatedImage string, lookup func(string) string) string {
	running := lookup(containerName)
	if running == "" || running == generatedImage {
		return ""
	}
	return fmt.Sprintf(
		"postgres image change detected for %q: running container uses %q, "+
			"this build generated %q — applying it will recreate postgres under "+
			"a different image. If %q is pgvector-based and this is unintended, "+
			"set POSTGRES_IMAGE=%s to keep it before running `nself start`",
		containerName, running, generatedImage, running, running,
	)
}
