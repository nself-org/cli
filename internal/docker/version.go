package docker

// Purpose: Docker daemon version and scoped restart commands for diagnostics.
// Inputs: a context and, for restart, a validated container name.
// Outputs: the server version or command error.
// Constraints: Docker subprocesses stay inside this package.
import (
	"context"
	"os/exec"
	"strings"
)

func ServerVersion(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}").Output()
	return strings.TrimSpace(string(out)), err
}

func RestartContainer(ctx context.Context, name string) error {
	return exec.CommandContext(ctx, "docker", "restart", name).Run()
}
