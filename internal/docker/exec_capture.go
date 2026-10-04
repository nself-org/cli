package docker

// exec_capture.go — run a command in a running container and capture its output.
//
// Purpose: non-interactive container commands whose result the CLI inspects
// (`nginx -t`, `nginx -s reload`), as opposed to Exec, which attaches the
// terminal.
// Inputs: a container name and the command with its arguments.
// Outputs: stdout and stderr as strings; an error quoting the last stderr
// line when the command exits non-zero.
// Constraints: no stdin, no TTY; all docker calls stay in this package.

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// ExecCapture runs `docker exec <container> <cmd...>` and returns the output.
func ExecCapture(ctx context.Context, container string, cmd []string) (stdout, stderr string, err error) {
	args := append([]string{"exec", container}, cmd...)
	c := exec.CommandContext(ctx, "docker", args...)
	var so, se bytes.Buffer
	c.Stdout, c.Stderr = &so, &se
	if runErr := c.Run(); runErr != nil {
		return so.String(), se.String(), fmt.Errorf("docker exec %s %v: %w: %s", container, cmd, runErr, lastLine(se.String()))
	}
	return so.String(), se.String(), nil
}
