package database

// Purpose: reads one variable from the RUNNING Hasura container's environment
// (`docker inspect`), for checks that must know what Hasura actually runs with
// rather than what the project config would give it on the next recreate.
// Inputs: context, *config.Config, variable name.
// Outputs: the value and whether the container defines it, or an error when
// the container cannot be inspected.
// Constraints: read-only; the command's output is never logged.

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// HasuraContainerEnvValue returns the value of key in the running Hasura
// container's environment. found is false when the container does not define it.
func HasuraContainerEnvValue(ctx context.Context, cfg *config.Config, key string) (value string, found bool, err error) {
	container := hasuraContainer(cfg)
	out, err := exec.CommandContext(ctx, "docker", "inspect", container,
		"--format", "{{range .Config.Env}}{{println .}}{{end}}").Output()
	if err != nil {
		return "", false, fmt.Errorf("docker inspect %s (is the stack running?): %w", container, err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), key+"="); ok {
			return v, true, nil
		}
	}
	return "", false, nil
}
