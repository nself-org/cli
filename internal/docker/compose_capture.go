package docker

// compose_capture.go — run a docker compose command and return its stdout.
//
// Purpose: commands that read a compose listing or a bounded log window (`logs --summary`,
// `logs --top`, the running-services list) need the output as bytes, which Compose.Run does not give
// on a non-TTY and Compose.ComposePs gives only as `ps --format json` (D-0749).
// Inputs: a context, the working directory and the arguments after "docker compose".
// Outputs: the child's stdout; an error quoting the last stderr line when it exits non-zero.
// Constraints: no stdin, no TTY; the call is bounded by ctx. A never-ending stream belongs to
// Stream or StreamLines.

import (
	"context"
)

// ComposeOutput runs `docker compose args...` in dir and returns what it wrote to stdout.
func ComposeOutput(ctx context.Context, dir string, args ...string) ([]byte, error) {
	c := NewProjectCompose(dir)
	if hasGlobalEnvFile(args) {
		c.EnvFiles = nil
	}
	return c.OutputProject(ctx, dir, ProjectRun{}, args...)
}
