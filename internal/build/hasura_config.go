package build

// Purpose: Generate {workdir}/hasura/config.yaml (the Hasura CLI's own project
//          config file, consumed by "hasura console"/"hasura metadata apply"
//          when a developer runs the hasura-cli binary directly) from the
//          resolved nSelf .env cascade, instead of shipping a static file with
//          a hardcoded endpoint/admin_secret that silently drifts from the
//          real project config (nself-cli-gaps-from-ntask-dogfood.md #10).
// Inputs:  workdir string — project root; cfg *config.Config — resolved config
//          (already loaded via the env cascade in Build()).
// Outputs: hasura/config.yaml written to disk (0600 — contains admin_secret).
// Constraints: Must be idempotent (same cfg -> byte-identical output) and a
//              no-op-safe overwrite so re-running `nself build` always keeps
//              the file in sync with the current .env cascade.
// SPORT: cli/internal/build — see gap #10 in
//        ~/Sites/nself/.claude/planning/nself-cli-gaps-from-ntask-dogfood.md

import (
	"fmt"
	"path/filepath"

	"github.com/nself-org/cli/internal/config"
)

// hasuraCLIConfigTemplate mirrors the Hasura CLI's own project config.yaml
// schema (https://hasura.io/docs/latest/hasura-cli/config-reference/).
// version: 3 is required for the metadata directory format used by
// applyViaHasuraCLI (internal/database/hasura.go).
const hasuraCLIConfigTemplate = `version: 3
endpoint: %s
admin_secret: "%s"
metadata_directory: metadata
actions:
  kind: synchronous
  handler_webhook_baseurl: %s
`

// RenderHasuraCLIConfig builds the Hasura CLI config.yaml contents from the
// resolved project config. endpoint is always the local Hasura URL — the
// Hasura CLI itself is meant to be run against whichever host the developer
// is on (local or via an SSH tunnel/port-forward to remote), matching the
// same "always localhost, tunnel to remote" convention already used by
// hasuraMetadataApplyCmd (internal/database/hasura.go).
func RenderHasuraCLIConfig(cfg *config.Config) []byte {
	port := cfg.Hasura.Port
	if port == 0 {
		port = 8080
	}
	endpoint := fmt.Sprintf("http://localhost:%d", port)

	actionsBaseURL := fmt.Sprintf("http://localhost:%d", port)
	if cfg.Functions.Port != 0 {
		actionsBaseURL = fmt.Sprintf("http://localhost:%d", cfg.Functions.Port)
	}

	return []byte(fmt.Sprintf(hasuraCLIConfigTemplate, endpoint, cfg.Hasura.AdminSecret, actionsBaseURL))
}

// WriteHasuraCLIConfig renders and writes {workdir}/hasura/config.yaml.
// Returns the number of files written (0 or 1) so callers can fold the count
// into the build's filesGenerated tally, matching the WriteLokiConfigs
// convention (internal/build/loki.go).
func WriteHasuraCLIConfig(workdir string, cfg *config.Config) (int, error) {
	return writeHasuraCLIConfigVia(newDiskSink(workdir), workdir, cfg)
}

// writeHasuraCLIConfigVia is WriteHasuraCLIConfig writing through sink.
func writeHasuraCLIConfigVia(sink Sink, workdir string, cfg *config.Config) (int, error) {
	hasuraDir := filepath.Join(workdir, "hasura")
	if err := sink.MkdirAll(hasuraDir, 0o755); err != nil {
		return 0, fmt.Errorf("creating hasura dir: %w", err)
	}

	content := RenderHasuraCLIConfig(cfg)
	path := filepath.Join(hasuraDir, "config.yaml")

	// Contains admin_secret — 0600, matching docker-compose.yml / .env* perms.
	if err := sink.WriteAtomic(path, content, 0o600); err != nil {
		return 0, fmt.Errorf("writing hasura/config.yaml: %w", err)
	}
	return 1, nil
}
