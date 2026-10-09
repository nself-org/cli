// installer.go — S2.T03: `nself bundle install <bundle>` orchestrator.
//
// Atomicity: pre-flight ALL licenses, then install each constituent plugin
// sequentially via plugin.Install. On any per-plugin failure we roll back by
// removing every plugin we successfully installed in THIS invocation. Plugins
// that were already on disk before the call are left untouched.
//
// --dry-run    prints the planned actions and exits without filesystem changes.
// --force      re-installs even if the same version is already installed
//
//	(repair/upgrade path). License is ALWAYS validated regardless of
//	--force; --force NEVER bypasses the license check.
//
// --strict     fails if any plugin in the bundle is missing from the registry
//
//	(default: skip missing with a warning).
//
// --channel    selects stable/beta/canary for version resolution (T22).
package bundle

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
	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/plugin"
)

// InstallOpts captures the user-controllable knobs for bundle install.
type InstallOpts struct {
	DryRun  bool
	Force   bool
	Strict  bool
	Channel Channel

	// Out is the stream to print human-facing progress messages.
	// nil → os.Stderr.
	Out io.Writer

	// PluginDir overrides the default plugin install location. Empty → use
	// the same resolution as cmd/commands/plugin.resolvePluginDir.
	PluginDir string

	// Config overrides the loaded project config. Optional — when nil the
	// caller-loaded cfg is used by Install via cfg arg.
	Config *config.Config

	// licenseChecker is an internal hook for tests to replace the Phase 2
	// (plugin-level) license validator. Production code leaves this nil.
	licenseChecker func(ctx context.Context, plugins []string) error

	// bundleEntitledChecker is an internal hook for tests to replace the Phase 1
	// (bundle-level) entitlement check that calls ping_api. Production code
	// leaves this nil and the installer uses license.BundleEntitled.
	bundleEntitledChecker func(ctx context.Context, key, bundleSlug string) (bool, error)

	// installer/remover are internal hooks for tests. Production code leaves
	// these nil and the installer uses plugin.Install / plugin.Remove.
	installer func(ctx context.Context, cfg *config.Config, name, pluginDir string) error
	remover   func(ctx context.Context, cfg *config.Config, name, pluginDir string) error
}

// InstallResult captures the outcome of a bundle install for callers that
// need to surface details (e.g. CR/QA harness, audit logger).
type InstallResult struct {
	Bundle        string
	Channel       Channel
	Planned       []string // plugins planned for install (post channel + strict filter)
	Skipped       []string // plugins skipped because missing from registry (non-strict)
	Installed     []string // plugins successfully installed in this call
	RolledBack    []string // plugins removed during rollback after a failure
	LicenseBypass bool     // reserved — always false; --force never bypasses license
	DryRun        bool
	Changed       bool // membership changed; the command layer must reconcile
}

// Install runs the bundle install flow. The returned InstallResult is populated
// even on error so callers (and tests) can inspect partial state.
func Install(ctx context.Context, bundleSlug string, opts InstallOpts) (*InstallResult, error) {
	out := opts.Out
	if out == nil {
		out = os.Stderr
	}

	b, ok := Get(bundleSlug)
	if !ok {
		return nil, UnknownBundleError(bundleSlug)
	}
	if !b.IsInstallable() {
		return nil, notInstallableError(b)
	}

	result := &InstallResult{
		Bundle:        b.Slug,
		Channel:       normalizeChannel(opts.Channel),
		LicenseBypass: false, // --force never bypasses license; this field is reserved for future internal-use audit
		DryRun:        opts.DryRun,
	}

	// Resolve plugin version pins via T22. Channel filter applied here.
	pins, err := ResolveBundleVersions(ctx, b.Slug, result.Channel)
	if err != nil {
		// Strict mode requires every plugin in the bundle to be in the registry.
		// Non-strict mode degrades to "install whatever IS in the registry".
		if opts.Strict {
			return result, fmt.Errorf("resolve bundle versions (strict): %w", err)
		}
		// Try a per-plugin probe to determine which plugins are missing.
		partialPins, partialSkipped, probeErr := probePartialBundle(ctx, b, result.Channel)
		if probeErr != nil {
			return result, fmt.Errorf("resolve bundle versions: %w", err)
		}
		pins = partialPins
		result.Skipped = partialSkipped
		if len(pins) == 0 {
			return result, fmt.Errorf("no plugins in bundle %q are eligible under channel %q", b.Slug, result.Channel)
		}
	}

	// Stable ordering for dry-run + install loop: walk b.Plugins, keep only
	// those resolved by pins.
	planned := make([]string, 0, len(pins))
	for _, name := range b.Plugins {
		if _, ok := pins[name]; ok {
			planned = append(planned, name)
		}
	}
	result.Planned = planned

	// Print the plan.
	printPlan(out, b, result.Channel, planned, pins, result.Skipped, opts)

	if opts.DryRun {
		_, _ = fmt.Fprintln(out, "(dry-run) no changes made.")
		return result, nil
	}

	// License pre-flight: two-phase check. Always runs — --force does NOT bypass.
	//   Phase 1 (bundle-level): call ping_api /bundle/entitled to verify the
	//   operator's key grants access to THIS bundle as a unit.
	//   Phase 2 (plugin-level fallback): per-plugin registry/license probe via
	//   defaultLicenseChecker to catch obvious missing-key cases before any FS
	//   change (mirrors existing plugin install behavior).
	{
		// Phase 1: bundle-level entitlement via ping_api (or test hook).
		key := license.CollectLicenseKey()
		entitledFn := opts.bundleEntitledChecker
		if entitledFn == nil {
			entitledFn = license.BundleEntitled
		}
		entitled, entitleErr := entitledFn(ctx, key, b.Slug)
		if entitleErr != nil || !entitled {
			msg := fmt.Sprintf("bundle %q: not entitled", b.Slug)
			if entitleErr != nil {
				msg = entitleErr.Error()
			}
			return result, fmt.Errorf("license validation failed: %s\n\nBuy at: https://nself.org/pricing or run 'nself license set <key>'", msg)
		}

		// Phase 2: plugin-level pre-flight (catches no-key-at-all cases for free).
		check := opts.licenseChecker
		if check == nil {
			check = defaultLicenseChecker
		}
		if err := check(ctx, planned); err != nil {
			return result, fmt.Errorf("license validation failed for bundle %q: %w", b.Slug, err)
		}
	}

	pluginDir := opts.PluginDir
	if pluginDir == "" {
		pluginDir = defaultPluginDir()
	}

	installFn := opts.installer
	if installFn == nil {
		installFn = func(ctx context.Context, cfg *config.Config, name, pd string) error {
			return plugin.Install(ctx, cfg, name, pd)
		}
	}
	removeFn := opts.remover
	if removeFn == nil {
		removeFn = func(ctx context.Context, cfg *config.Config, name, pd string) error {
			return plugin.Remove(ctx, cfg, name, pd, false, true)
		}
	}

	cfg := opts.Config
	if cfg == nil {
		loaded, lerr := loadConfigOrDefault()
		if lerr != nil {
			return result, fmt.Errorf("loading project config: %w", lerr)
		}
		cfg = loaded
	}

	// Build a lookup of currently installed plugin versions for downgrade detection.
	existingVersions := buildInstalledVersionMap(pluginDir)
	backups := make(map[string]string)
	backupRoot := ""
	rollback := func() error {
		result.RolledBack = rollbackInstalled(ctx, cfg, removeFn, pluginDir, result.Installed, out)
		restoreErr := restoreBundleUpgrades(pluginDir, backupRoot, backups, out)
		result.Changed = !maps.Equal(existingVersions, buildInstalledVersionMap(pluginDir)) || restoreErr != nil
		return restoreErr
	}

	// Sequential install with rollback on failure.
	for _, name := range planned {
		targetVer := string(pins[name])
		existingVer, alreadyExists := existingVersions[strings.ToLower(name)]

		if alreadyExists {
			cmp := plugin.CompareVersions(existingVer, targetVer)
			if cmp == 0 {
				// Same version: skip silently.
				_, _ = fmt.Fprintf(out, "  ✓ %s@%s (already installed, same version)\n", name, targetVer)
				continue
			}
			if cmp > 0 {
				// Existing is HIGHER than what the bundle pins.
				if opts.Strict {
					// --strict: fail if any plugin already at a higher version.
					rollbackErr := rollback()
					return result, errors.Join(fmt.Errorf("--strict: plugin %q is at %s (higher than bundle pin %s); use without --strict to skip", name, existingVer, targetVer), rollbackErr)
				}
				// Default: skip with warning, no silent downgrade.
				_, _ = fmt.Fprintf(out, "  ⚠ %s: installed at %s (higher than bundle pin %s) — skipping to avoid downgrade\n", name, existingVer, targetVer)
				continue
			}
			// cmp < 0: existing is lower → upgrade, fall through to install.
			_, _ = fmt.Fprintf(out, "  ↑ upgrading %s: %s → %s\n", name, existingVer, targetVer)
			if backupRoot == "" {
				backupRoot, err = os.MkdirTemp(pluginDir, ".bundle-rollback-")
				if err != nil {
					rollbackErr := rollback()
					return result, errors.Join(fmt.Errorf("creating bundle upgrade backup: %w", err), rollbackErr)
				}
			}
			backup := filepath.Join(backupRoot, name)
			if err := os.Rename(filepath.Join(pluginDir, name), backup); err != nil {
				rollbackErr := rollback()
				return result, errors.Join(fmt.Errorf("backing up plugin %q: %w", name, err), rollbackErr)
			}
			backups[name] = backup
		} else {
			_, _ = fmt.Fprintf(out, "  → installing %s@%s...\n", name, targetVer)
		}

		if err := installFn(ctx, cfg, name, pluginDir); err != nil {
			_, _ = fmt.Fprintf(out, "  ✗ %s install failed: %v\n", name, err)
			rollbackErr := rollback()
			return result, errors.Join(fmt.Errorf("bundle %q install failed at plugin %q: %w", b.Slug, name, err), rollbackErr)
		}
		result.Installed = append(result.Installed, name)
		result.Changed = true
		_, _ = fmt.Fprintf(out, "  ✓ %s@%s installed\n", name, targetVer)
	}
	if backupRoot != "" {
		if err := os.RemoveAll(backupRoot); err != nil {
			return result, fmt.Errorf("removing bundle upgrade backup: %w", err)
		}
	}

	_, _ = fmt.Fprintf(out, "\nBundle %q (%s) installed: %d plugins.\n", b.Name, b.Slug, len(result.Installed))
	if len(result.Skipped) > 0 {
		_, _ = fmt.Fprintf(out, "Skipped (missing from registry): %s\n", strings.Join(result.Skipped, ", "))
	}

	return result, nil
}
