package bundle

// installer_helpers.go — supporting helpers for the bundle install orchestrator.
//
// Purpose: partial-install detection/rollback, dry-run plan printing and small config/plugin-dir lookups used by Install and InstallMultiple in installer.go, split out for file size.
// Inputs: an InstallOpts and the plugin/bundle state on disk.
// Outputs: rollback actions, printed plan output, or looked-up config values. The licence pre-flight is in installer_helpers_licence.go.
// Constraints: pure move from installer.go (CLI-R12 Batch E); no behaviour change. License is still always validated regardless of --force.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/plugin"
)

// probePartialBundle resolves bundle plugins one-by-one so that a single
// missing plugin does not abort the whole bundle in non-strict mode.
func probePartialBundle(ctx context.Context, b Bundle, ch Channel) (VersionPins, []string, error) {
	pins := make(VersionPins, len(b.Plugins))
	var skipped []string
	reg, err := plugin.FetchRegistry(ctx, "", defaultRegistryCacheDir())
	if err != nil {
		return nil, nil, err
	}
	for _, name := range b.Plugins {
		v, vErr := resolveOnePluginVersion(reg, name, ch)
		if vErr != nil {
			skipped = append(skipped, name)
			continue
		}
		pins[name] = v
	}
	return pins, skipped, nil
}

// rollbackInstalled walks installed in reverse and removes each plugin. Errors
// during rollback are logged but never propagated — best-effort cleanup.
func rollbackInstalled(
	ctx context.Context,
	cfg *config.Config,
	remove func(ctx context.Context, cfg *config.Config, name, pluginDir string) error,
	pluginDir string,
	installed []string,
	out io.Writer,
) []string {
	rolled := make([]string, 0, len(installed))
	for i := len(installed) - 1; i >= 0; i-- {
		name := installed[i]
		_, _ = fmt.Fprintf(out, "  ↩ rolling back %s...\n", name)
		if err := remove(ctx, cfg, name, pluginDir); err != nil {
			_, _ = fmt.Fprintf(out, "  ! rollback of %s failed: %v (manual cleanup may be required)\n", name, err)
			continue
		}
		rolled = append(rolled, name)
	}
	return rolled
}

// restoreBundleUpgrades puts pre-existing plugin directories back after a
// failed bundle install. A failed restore retains its backup for manual repair.
func restoreBundleUpgrades(pluginDir, backupRoot string, backups map[string]string, out io.Writer) error {
	var restoreErr error
	for name, backup := range backups {
		current := filepath.Join(pluginDir, name)
		if err := os.RemoveAll(current); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("removing failed upgrade %q: %w", name, err))
			continue
		}
		if err := os.Rename(backup, current); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("restoring plugin %q from %s: %w", name, backup, err))
			continue
		}
		_, _ = fmt.Fprintf(out, "  ↩ restored previous version of %s\n", name)
	}
	if restoreErr == nil && backupRoot != "" {
		if err := os.RemoveAll(backupRoot); err != nil {
			restoreErr = fmt.Errorf("removing bundle rollback backup %s: %w", backupRoot, err)
		}
	}
	return restoreErr
}

// printPlan dumps a human-readable summary of what install will do.
func printPlan(out io.Writer, b Bundle, ch Channel, planned []string, pins VersionPins, skipped []string, opts InstallOpts) {
	header := "Install plan"
	if opts.DryRun {
		header = "Install plan (dry-run)"
	}
	_, _ = fmt.Fprintf(out, "%s for bundle %s (%s) — channel %s\n", header, b.Name, b.Slug, ch)
	if len(planned) == 0 {
		_, _ = fmt.Fprintln(out, "  (no plugins to install)")
	}
	for _, name := range planned {
		_, _ = fmt.Fprintf(out, "  • %s@%s\n", name, pins[name])
	}
	if len(skipped) > 0 {
		_, _ = fmt.Fprintf(out, "  skipped (not in registry, non-strict): %s\n", strings.Join(skipped, ", "))
	}
	if b.IsFree() {
		_, _ = fmt.Fprintln(out, "  license: free bundle (no bundle entitlement call; each plugin's licence is still checked)")
	} else if opts.Force {
		_, _ = fmt.Fprintln(out, "  license: will validate (--force only skips same-version check)")
	} else {
		_, _ = fmt.Fprintln(out, "  license: will validate each plugin before any FS change")
	}
}

// notInstallableError builds an actionable error for a bundle that
// IsInstallable() rejected. Since P7-PLUG-19 only the ɳSelf+ meta-bundle (no SKU
// of its own: it is the union of every paid bundle's entitlements) and a bundle
// with no plugins are refused; the free Task Bundle installs as a unit. Rather
// than a bare "not installable" message, name what the operator should run
// instead so the refusal is actionable, not a dead end.
func notInstallableError(b Bundle) error {
	if b.Slug == selfPlusSlug {
		return fmt.Errorf(
			"bundle %q is not installable as a unit: ɳSelf+ is a meta-bundle (the union of every paid bundle's entitlements, not its own SKU).\nInstall one of the paid bundles directly instead, e.g. 'nself bundle install claw', or install individual plugins via 'nself plugin install <name>'",
			b.Slug,
		)
	}
	return fmt.Errorf("bundle %q is not installable as a unit: it has no plugins", b.Slug)
}

// isAlreadyInstalled reports whether the plugin's directory exists in pluginDir.
// Used by both installer and remover for a fast existence probe.
func isAlreadyInstalled(pluginDir, name string) bool {
	_, err := os.Stat(filepath.Join(pluginDir, name))
	return err == nil
}

// buildInstalledVersionMap returns a map of lowercase plugin name → installed
// version string for all plugins found in pluginDir. Plugins with no version
// information get an empty string (treated as "0.0.0" by callers).
func buildInstalledVersionMap(pluginDir string) map[string]string {
	m := make(map[string]string)
	installed, err := plugin.ListInstalled(pluginDir)
	if err != nil {
		return m
	}
	for _, p := range installed {
		m[strings.ToLower(p.Name)] = p.Version
	}
	return m
}

// defaultPluginDir resolves the standard plugin install directory. Mirrors
// cmd/commands/plugin.go's resolvePluginDir so this package does not import
// the cmd layer.
func defaultPluginDir() string {
	if d := os.Getenv("NSELF_PLUGIN_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/tmp", ".nself", "plugins")
	}
	return filepath.Join(home, ".nself", "plugins")
}

// loadConfigOrDefault loads the project config from cwd. On error (e.g. no
// .env files yet) we return a zero-value Config so plugin.Install can proceed
// using its own defaults. This mirrors plugin install behavior outside a project.
func loadConfigOrDefault() (*config.Config, error) {
	wd, err := os.Getwd()
	if err != nil {
		return &config.Config{}, nil
	}
	cfg, err := config.Load(wd)
	if err != nil {
		// Bundle install must work outside a project too (CI, fresh install).
		return &config.Config{}, nil
	}
	return cfg, nil
}
