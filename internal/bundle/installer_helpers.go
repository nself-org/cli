package bundle

// installer_helpers.go — supporting helpers for the bundle install orchestrator.
//
// Purpose: partial-install detection/rollback, dry-run plan printing, license checking and small config/plugin-dir lookups used by Install and InstallMultiple in installer.go, split out for file size.
// Inputs: an InstallOpts and the plugin/bundle state on disk.
// Outputs: rollback actions, printed plan output, or looked-up config/license values.
// Constraints: pure move from installer.go (CLI-R12 Batch E); no behaviour change. License is still always validated regardless of --force.

import (
	"context"
	"errors"
	"fmt"
	"github.com/nself-org/cli/internal/license"
	"io"
	"os"
	"os/exec"
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
	if opts.Force {
		_, _ = fmt.Fprintln(out, "  license: will validate (--force only skips same-version check)")
	} else {
		_, _ = fmt.Fprintln(out, "  license: will validate each plugin before any FS change")
	}
}

// notInstallableError builds an actionable error for a bundle that
// IsInstallable() rejected. The ɳSelf+ meta-bundle and the free Task Bundle
// are deliberately not installable as a unit (design confirmed, Ruling 2 +
// P6-E4-W3-S3-T10): ɳSelf+ has no SKU of its own (it's the union of every
// paid bundle's entitlements) and free bundles need no license, so their
// plugins install directly via `nself plugin install`, one at a time, with
// zero license gate. Rather than a bare "not installable" message, name the
// exact commands the operator should run instead so the refusal is
// actionable, not a dead end.
func notInstallableError(b Bundle) error {
	if b.Slug == selfPlusSlug {
		return fmt.Errorf(
			"bundle %q is not installable as a unit: ɳSelf+ is a meta-bundle (the union of every paid bundle's entitlements, not its own SKU).\nInstall one of the paid bundles directly instead, e.g. 'nself bundle install claw', or install individual plugins via 'nself plugin install <name>'",
			b.Slug,
		)
	}
	if len(b.Plugins) == 0 {
		return fmt.Errorf("bundle %q is not installable as a unit: it has no plugins", b.Slug)
	}
	cmds := make([]string, 0, len(b.Plugins))
	for _, p := range b.Plugins {
		cmds = append(cmds, "  nself plugin install "+p)
	}
	return fmt.Errorf(
		"bundle %q is not installable as a unit: it is a free bundle, and its plugins need no license — install each one directly instead:\n%s",
		b.Slug, strings.Join(cmds, "\n"),
	)
}

// defaultLicenseChecker validates every paid plugin in the bundle has license
// coverage BEFORE the installer touches the filesystem. Implementation
// strategy: rely on plugin.Install's internal license gate by NOT bypassing,
// and pre-check via a lightweight cache probe. Production-grade validation
// occurs inside plugin.Install for each plugin; this pre-flight surfaces
// the failure earlier and lets us short-circuit before partial install.
//
// We do this by calling plugin.CheckEOLBlock as a registry-fetch probe (cheap)
// and inspecting the manifest. If any plugin requires a license and no key is
// configured, we return an error here.
func defaultLicenseChecker(ctx context.Context, plugins []string) error {
	// Quick sanity: at least one license key must exist when any plugin
	// requires a license. plugin.Install does the deep validation per-plugin;
	// this pre-flight just blocks an empty-key obvious miss.
	if len(plugins) == 0 {
		return nil
	}
	// Use plugin.FetchRegistry to look up which plugins require licenses.
	reg, err := plugin.FetchRegistry(ctx, "", defaultRegistryCacheDir())
	if err != nil {
		// Network failure: defer to plugin.Install's offline cache logic.
		return nil
	}
	manifests := make(map[string]plugin.PluginManifest, len(plugins))
	for _, m := range reg.Plugins {
		manifests[strings.ToLower(m.Name)] = m
	}
	for _, name := range plugins {
		m, ok := manifests[strings.ToLower(name)]
		if !ok {
			// Missing from registry — plugin.Install will fail naturally;
			// don't pre-error here so we get the proper "not found" message.
			continue
		}
		// Free plugins don't need a license. The resolved manifest's own
		// requires_license field is authoritative; the paidPlugins name map
		// that used to be AND-ed in here is a pre-registry fallback that has
		// drifted, and for a tier_pair slug (cron, notify) it reports "paid"
		// even when the entry actually being installed is the free one —
		// which made a wholly free bundle demand a license key.
		if !m.RequiresLicense {
			continue
		}
		// Paid plugin: ensure at least one license key is set. We don't
		// validate ENTITLEMENTS here — plugin.Install does that per-plugin
		// with cache + remote fallback. We're just catching the obvious
		// "no key at all" case so the user sees one error, not N.
		if !hasAnyLicenseKey() {
			return errors.New("bundle requires at least one paid plugin; no license key configured. Run 'nself license set <key>' or visit nself.org/pricing")
		}
		// One paid plugin with a key present is enough to proceed; the
		// per-plugin validator inside plugin.Install runs full checks.
		break
	}
	return nil
}

// hasAnyLicenseKey returns true if the operator has any license key set
// (env, cache, or legacy key file). Real validation is left to plugin.Install.
func hasAnyLicenseKey() bool {
	// license.CollectLicenseKeys is the canonical accessor: it reads
	// NSELF_PLUGIN_LICENSE_KEY, the numbered NSELF_LICENSE_KEY_1..10 vars, and
	// the stored key at ~/.nself/license/key — the exact file that
	// `nself license set` writes.
	//
	// This used to hand-roll the lookup and checked ~/.nself/license.key, a
	// LEGACY path with a dot where the current store has a directory
	// separator. The effect was that `nself license set <key>` followed by
	// `nself bundle install <paid bundle>` reported "no license key
	// configured" while `nself license status` showed the same key as Active.
	// Every paying customer hit that. Never re-derive the key locations here.
	if len(license.CollectLicenseKeys()) > 0 {
		return true
	}
	// The owner all-access key is a vault convenience the canonical collector
	// does not know about.
	return os.Getenv("NSELF_PLUGIN_LICENSE_KEY_OWNER") != ""
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

// triggerBuild invokes `nself build` once at the end of a bundle install so
// docker-compose.yml and nginx configs are regenerated with the new plugins.
// This is a subprocess call matching the pattern in internal/promote/promote.go.
func triggerBuild(ctx context.Context, out io.Writer) error {
	_, _ = fmt.Fprintln(out, "\nRunning 'nself build' to apply installed plugins...")
	nself, err := exec.LookPath("nself")
	if err != nil {
		// nself binary not found — likely running in tests or a non-standard PATH.
		// Allow callers to detect this via the error message.
		return fmt.Errorf("nself binary not found in PATH: %w", err)
	}
	cmd := exec.CommandContext(ctx, nself, "build", "--quiet")
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nself build: %w", err)
	}
	return nil
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
