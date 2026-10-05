package simharness

// Purpose: the one place simharness runs a docker process.
// Inputs: a context and a docker argv (no shell, no string building).
// Outputs: stdout, or an error that carries the argv and stderr.
// Constraints: every exec.Command in the package lives here; tests replace commandContext to assert argv.

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// commandContext is the test hook for argv assertions.
var commandContext = exec.CommandContext

// dockerTimeout bounds every docker call except image builds.
const dockerTimeout = 2 * time.Minute

// docker runs `docker args...` and returns stdout (trimmed of the final newline).
func docker(ctx context.Context, args ...string) (string, error) {
	return dockerIn(ctx, "", args...)
}

// dockerIn is docker with stdin.
func dockerIn(ctx context.Context, stdin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, dockerTimeout)
	defer cancel()
	return run(ctx, stdin, args)
}

// run executes docker without adding a timeout (image builds set their own).
func run(ctx context.Context, stdin string, args []string) (string, error) {
	cmd := commandContext(ctx, "docker", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimRight(out.String(), "\n"), nil
}
