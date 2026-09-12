package config

import (
	"fmt"
	"strings"
)

// Purpose: path/volume validation helpers for CS_N custom-service env vars,
// split out of custom_services.go to keep that file's parse loop readable.
// Shared by CS_N_PATH, CS_N_ENV_FILE (both single relative paths) and
// CS_N_VOLUMES (comma-separated host:container[:mode] triples whose host
// half is checked the same way). Extracted once a third caller needed the
// same traversal check (G-013), per the repo's DRY-on-third-copy convention.
// Inputs: raw string values read directly from os.Getenv by the caller.
// Outputs: nil on a safe value, otherwise an error naming the problem —
// callers wrap it with the specific CS_N_* var name for context.
// Constraints: intentionally permissive on everything except escaping the
// project root — these are operator-authored env vars, not untrusted input,
// so the goal is catching mistakes, not adversarial hardening.

// validateRelativePath rejects an absolute path or one containing a ".."
// path-traversal segment. Used for any CS_N_* value that names a location
// inside the project tree (CS_N_PATH, CS_N_ENV_FILE).
func validateRelativePath(p string) error {
	if strings.HasPrefix(p, "/") {
		return fmt.Errorf("must be a relative path, got %q", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return fmt.Errorf("must not contain '..', got %q", p)
		}
	}
	return nil
}

// validateCustomServiceVolumes checks a CS_N_VOLUMES value: a comma-separated
// list of "host:container[:mode]" entries. Each entry must have at least a
// host and container path; a relative host path (one not starting with "/"
// and not a bare named-volume identifier containing no "/") is checked for
// traversal via validateRelativePath. Named Docker volumes (e.g.
// "my_data:/data") and absolute bind mounts (e.g. "/srv/x:/data") are left to
// the operator's judgment, matching Docker Compose's own permissive stance.
func validateCustomServiceVolumes(raw string) error {
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return fmt.Errorf("contains an empty entry in %q", raw)
		}
		parts := strings.Split(entry, ":")
		if len(parts) < 2 {
			return fmt.Errorf("entry %q must be host:container[:mode]", entry)
		}
		host := parts[0]
		if host == "" {
			return fmt.Errorf("entry %q is missing a host path", entry)
		}
		// Only check paths that look like a project-relative bind mount
		// ("./x", "../x", or a bare relative segment containing "/").
		// Absolute paths and bare named-volume names (no "/") are exempt.
		if !strings.HasPrefix(host, "/") && strings.Contains(host, "/") {
			if err := validateRelativePath(host); err != nil {
				return fmt.Errorf("entry %q: %w", entry, err)
			}
		}
	}
	return nil
}
