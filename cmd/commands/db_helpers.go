package commands

// Purpose: shared helpers used across db subcommands: loading the tenant
// allowlist (from config or YAML) and loading the project config. Inputs are
// raw config/YAML bytes; outputs are parsed allowlist entries or a *config.Config.
// Constraints: split out of db.go (CLI-R12) as a pure move, no behavior change.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/tenant"
)

// loadAllowlist reads the RLS allowlist from the project working directory.
func loadAllowlist(_ *config.Config) []tenant.AllowlistEntry {
	dir, _ := os.Getwd()
	paths := []string{
		filepath.Join(dir, "lint_allowlist.yaml"),
		filepath.Join(dir, ".nself", "lint_allowlist.yaml"),
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		return parseAllowlistYAML(data)
	}
	return nil
}

// parseAllowlistYAML parses a simple YAML allowlist. Format:
//
//	tables:
//	  - schema: public
//	    table: migrations
//	    reason: "System metadata table"
func parseAllowlistYAML(data []byte) []tenant.AllowlistEntry {
	var entries []tenant.AllowlistEntry
	lines := strings.Split(string(data), "\n")
	var current *tenant.AllowlistEntry
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- schema:") {
			if current != nil {
				entries = append(entries, *current)
			}
			current = &tenant.AllowlistEntry{
				Schema: strings.TrimSpace(strings.TrimPrefix(trimmed, "- schema:")),
			}
		} else if strings.HasPrefix(trimmed, "table:") && current != nil {
			current.Table = strings.TrimSpace(strings.TrimPrefix(trimmed, "table:"))
		} else if strings.HasPrefix(trimmed, "reason:") && current != nil {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "reason:"))
			current.Reason = strings.Trim(val, "\"'")
		}
	}
	if current != nil {
		entries = append(entries, *current)
	}
	return entries
}

// ── init ────────────────────────────────────────────────────────────

// loadProjectConfig loads the nSelf configuration from the current working directory.
func loadProjectConfig() (*config.Config, error) {
	return loadProjectConfigWithOptions(config.LoadOptions{})
}

// loadProjectConfigWithOptions loads the same cascade with explicit caller scope.
func loadProjectConfigWithOptions(opts config.LoadOptions) (*config.Config, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}
	cfg, err := config.LoadWithOptions(dir, opts)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	return cfg, nil
}

// ── run functions ───────────────────────────────────────────────────
