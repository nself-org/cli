package config

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolvePluginDir selects an explicit caller override, the environment,
// configured directory, or ~/.nself/plugins, and expands home shorthand.
// Inputs: configured PLUGIN_DIR (empty without a project). Outputs: the plugin
// directory, falling back to /tmp without a home. Constraints: no disk writes.
func ResolvePluginDir(configured string, explicit ...string) string {
	if len(explicit) > 0 && explicit[0] != "" {
		return ExpandHomeDir(explicit[0])
	}
	dir := strings.TrimSpace(os.Getenv("NSELF_PLUGIN_DIR"))
	if dir == "" {
		dir = configured
	}
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			home = "/tmp"
		}
		return filepath.Join(home, ".nself", "plugins")
	}
	return ExpandHomeDir(dir)
}

// ExpandHomeDir replaces only a leading ~ or ~/ with the user's home.
// Inputs/Outputs: a path. Constraints: unknown users and missing homes stay
// literal; no shell expansion or filesystem access is performed.
func ExpandHomeDir(dir string) string {
	if dir != "~" && !strings.HasPrefix(dir, "~/") {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return dir
	}
	return filepath.Join(home, strings.TrimPrefix(dir, "~"))
}

// CertificateDirName maps a domain to the directory shared by cert writers and
// readers. Inputs: domain. Outputs: dots replaced by dashes, or localhost for
// an empty domain. Constraints: no filesystem access.
func CertificateDirName(domain string) string {
	if domain == "" {
		return "localhost"
	}
	return strings.ReplaceAll(domain, ".", "-")
}
