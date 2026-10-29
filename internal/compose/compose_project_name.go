package compose

// Purpose: warn when the project .env names a compose project other than PROJECT_NAME (D-0448).
// Inputs: the project workdir and the loaded PROJECT_NAME.
// Outputs: a one-line warning, or "" when there is nothing to say.
// Constraints: read-only; names the volumes, never a value from the file besides the two names.
//
// The bash-era compose file used `name: ${COMPOSE_PROJECT_NAME:-${PROJECT_NAME}}`. The Go CLI
// writes COMPOSE_PROJECT_NAME=PROJECT_NAME to .nself/compose.env, which docker compose reads
// after .env, so the compose project (and every volume prefix) is always PROJECT_NAME. A
// project whose .env set a different COMPOSE_PROJECT_NAME starts on new, empty volumes.

import (
	"fmt"
	"github.com/nself-org/cli/internal/config"
	"path/filepath"
	"strings"
)

// ProjectNameWarning reads COMPOSE_PROJECT_NAME from workdir/.env, the only project
// env file compose reads (ComposeEnvFiles), and compares it with projectName.
func ProjectNameWarning(workdir, projectName string) string {
	env, err := config.ReadEnvFile(filepath.Join(workdir, ".env"))
	if err != nil {
		return ""
	}
	legacy := strings.TrimSpace(env["COMPOSE_PROJECT_NAME"])
	if legacy == "" || legacy == projectName {
		return ""
	}
	return fmt.Sprintf("COMPOSE_PROJECT_NAME=%s in .env differs from PROJECT_NAME=%s: nself runs docker compose as project %q, so named volumes are %s_postgres_data and so on. "+
		"If this stack ran earlier as %q, its data is in %s_postgres_data and the other %s_* volumes, and the new containers start empty. "+
		"Set PROJECT_NAME=%s to keep using them, or remove COMPOSE_PROJECT_NAME from .env if the %s_* volumes are the ones you want",
		legacy, projectName, projectName, projectName, legacy, legacy, legacy, legacy, projectName)
}
