package compose

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

// Purpose: filesystem/parsing helpers for the CS_N_ENV_FILE and CS_N_VOLUMES
// custom-service extensions (G-013). Split out of custom_services.go so that
// file keeps its focus on the fixed env-var set and the ServiceConfig
// builders.
// Inputs: a project-relative path (CS_N_ENV_FILE) or a raw CS_N_VOLUMES
// string, plus the Generator's workDir for resolving the former on disk.
// Outputs: a parsed env map or volume-mount slice ready to attach to a
// ServiceConfig.
// Constraints: CS_N_ENV_FILE's path traversal/absolute-path safety was
// already checked by config.parseCustomServices — this layer only resolves
// and reads it. CS_N_VOLUMES entries were similarly pre-validated; this
// layer only splits them into the []string form ServiceConfig.Volumes wants.

// loadCustomServiceEnvFile reads a dotenv-format file named by CS_N_ENV_FILE
// and returns its KEY=VALUE pairs. workDir anchors the (already-validated,
// project-relative) path; an empty workDir falls back to resolving relative
// to the process's current directory, matching how CS_N_PATH build contexts
// are implicitly resolved when no explicit project root is threaded through.
func loadCustomServiceEnvFile(workDir, relPath string) (map[string]string, error) {
	path := relPath
	if workDir != "" {
		path = filepath.Join(workDir, relPath)
	}
	vars, err := godotenv.Read(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return vars, nil
}

// parseCustomServiceVolumes splits a CS_N_VOLUMES value ("host:container[:mode]"
// entries, comma-separated) into the []string form docker-compose's `volumes:`
// list expects. Returns nil for an empty input so ServiceConfig.Volumes stays
// unset (omitempty) rather than an empty-but-present list.
func parseCustomServiceVolumes(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			out = append(out, entry)
		}
	}
	return out
}
