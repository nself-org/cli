package bundle

// installer_members.go — the sequential member install loop with rollback.
//
// Purpose: install each planned plugin in order; on any failure remove what this
// invocation installed and restore upgraded plugins from their backups.
// Inputs: a memberRun holding the plan, the plugin directory and the install and
// remove functions.
// Outputs: result.Installed / RolledBack / Changed, and the first error.
// Constraints: moved from installer.go (P7-PLUG-19, file size); behaviour
// unchanged. Plugins already on disk at the bundle's version are left untouched;
// a higher installed version is kept (no silent downgrade).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/plugin"
)

type memberRun struct {
	b         Bundle
	opts      InstallOpts
	out       io.Writer
	result    *InstallResult
	planned   []string
	pins      VersionPins
	pluginDir string
	cfg       *config.Config
	installFn func(ctx context.Context, cfg *config.Config, name, pluginDir string) error
	removeFn  func(ctx context.Context, cfg *config.Config, name, pluginDir string) error
}

func (m *memberRun) run(ctx context.Context) error {
	out, result, pluginDir := m.out, m.result, m.pluginDir
	// Build a lookup of currently installed plugin versions for downgrade detection.
	existingVersions := buildInstalledVersionMap(pluginDir)
	backups := make(map[string]string)
	backupRoot := ""
	rollback := func() error {
		result.RolledBack = rollbackInstalled(ctx, m.cfg, m.removeFn, pluginDir, result.Installed, out)
		restoreErr := restoreBundleUpgrades(pluginDir, backupRoot, backups, out)
		result.Changed = !maps.Equal(existingVersions, buildInstalledVersionMap(pluginDir)) || restoreErr != nil
		return restoreErr
	}

	// Sequential install with rollback on failure.
	for _, name := range m.planned {
		targetVer := string(m.pins[name])
		existingVer, alreadyExists := existingVersions[strings.ToLower(name)]

		if alreadyExists {
			cmp := plugin.CompareVersions(existingVer, targetVer)
			if cmp == 0 {
				// Same version: skip silently.
				_, _ = fmt.Fprintf(out, "  ✓ %s@%s (already installed, same version)\n", name, targetVer)
				m.noteKeptFree(ctx, name)
				continue
			}
			if cmp > 0 {
				// Existing is HIGHER than what the bundle pins.
				if m.opts.Strict {
					// --strict: fail if any plugin already at a higher version.
					rollbackErr := rollback()
					return errors.Join(fmt.Errorf("--strict: plugin %q is at %s (higher than bundle pin %s); use without --strict to skip", name, existingVer, targetVer), rollbackErr)
				}
				// Default: skip with warning, no silent downgrade.
				_, _ = fmt.Fprintf(out, "  ⚠ %s: installed at %s (higher than bundle pin %s) — skipping to avoid downgrade\n", name, existingVer, targetVer)
				m.noteKeptFree(ctx, name)
				continue
			}
			// cmp < 0: existing is lower → upgrade, fall through to install.
			_, _ = fmt.Fprintf(out, "  ↑ upgrading %s: %s → %s\n", name, existingVer, targetVer)
			if backupRoot == "" {
				var err error
				backupRoot, err = os.MkdirTemp(pluginDir, ".bundle-rollback-")
				if err != nil {
					rollbackErr := rollback()
					return errors.Join(fmt.Errorf("creating bundle upgrade backup: %w", err), rollbackErr)
				}
			}
			backup := filepath.Join(backupRoot, name)
			if err := os.Rename(filepath.Join(pluginDir, name), backup); err != nil {
				rollbackErr := rollback()
				return errors.Join(fmt.Errorf("backing up plugin %q: %w", name, err), rollbackErr)
			}
			backups[name] = backup
		} else {
			_, _ = fmt.Fprintf(out, "  → installing %s@%s...\n", name, targetVer)
		}

		if err := m.installFn(ctx, m.cfg, name, pluginDir); err != nil {
			_, _ = fmt.Fprintf(out, "  ✗ %s install failed: %v\n", name, err)
			rollbackErr := rollback()
			return errors.Join(fmt.Errorf("bundle %q install failed at plugin %q: %w", m.b.Slug, name, err), rollbackErr)
		}
		result.Installed = append(result.Installed, name)
		result.Changed = true
		_, _ = fmt.Fprintf(out, "  ✓ %s@%s installed\n", name, targetVer)
	}
	if backupRoot != "" {
		if err := os.RemoveAll(backupRoot); err != nil {
			return fmt.Errorf("removing bundle upgrade backup: %w", err)
		}
	}
	return nil
}
