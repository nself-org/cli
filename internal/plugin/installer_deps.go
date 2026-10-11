package plugin

// installer_deps.go — Step 3 of installLocked: validate and install dependencies.
//
// Purpose: validate every dependency name and binary name, then install each
// missing dependency, before installLocked downloads the plugin itself. Split
// out of installer_locked.go for the 300-line cap (P7-PLUG-20).
// Inputs: the plugin name, its registry manifest, target config and pluginDir.
// Outputs: nil once every dependency is on disk, or the first error.
// Constraints: pure move; the install lock is already held by the caller, so
// dependencies install through installLocked, never Install (that would deadlock).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// installDependencies validates and installs the dependencies of name while the
// install lock is held.
func installDependencies(ctx context.Context, cfg *config.Config, name string, manifest *PluginManifest, pluginDir string) error {
	deps := manifest.Dependencies
	// Validate every dependency name and every binary name before any join, stat or
	// install: an existing sibling dir named "../victim" must not skip the install
	// loop, and a registry entry must not reach a path outside pluginDir (P7-PLUG-20).
	for _, dep := range deps {
		if err := ValidateName(dep); err != nil {
			return fmt.Errorf("plugin %q lists an invalid dependency: %w", name, err)
		}
	}
	if err := validateCLIBinaries(name, manifest); err != nil {
		return fmt.Errorf("plugin %q: %w", name, err)
	}
	if len(deps) > 0 {
		fmt.Fprintf(os.Stderr, "Installing %s (requires: %s)\n", name, strings.Join(deps, ", "))
	}
	for _, dep := range deps {
		depDir := filepath.Join(pluginDir, dep)
		if _, err := os.Stat(depDir); err == nil {
			fmt.Fprintf(os.Stderr, "  ✓ %s (already installed)\n", dep)
			continue // dependency already installed
		}
		fmt.Fprintf(os.Stderr, "  → Installing dependency %s...\n", dep)
		if err := installLocked(ctx, cfg, dep, pluginDir); err != nil {
			return fmt.Errorf("installing dependency %q: %w", dep, err)
		}
		fmt.Fprintf(os.Stderr, "  ✓ %s installed\n", dep)
	}
	return nil
}
