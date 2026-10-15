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
//
// App-ready (P7-PLUG-19): after the members are installed the installer records
// the ledger and, when InstallOpts.Hooks is set, runs reconcile, start, readiness,
// tracking and health (package appready; installer_appready.go). The member loop
// is in installer_members.go and the licence pre-flight in installer_licence.go.
// Operation lock (D-0322): everything after the licence pre-flight runs under the
// ledger store's operation lock (ledger/oplock.go), so a second install, a remove
// or any ledger writer waits with progress instead of failing at the ledger lock.
// A free bundle (Bundle.IsFree) skips the ping_api bundle entitlement call.
package bundle

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/bundle/appready"
	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/config"
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

	// Hooks, when non-nil, make Install run reconcile, start, readiness,
	// tracking and health after the ledger step and judge app-ready. Nil keeps
	// the legacy behaviour: install, ledger, and the caller reconciles.
	Hooks *appready.Hooks

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

	// undoer reverses a fresh member install after a failed reconcile (D-0760).
	// Production code leaves it nil and the installer uses plugin.UndoInstall,
	// which never drops a schema.
	undoer func(ctx context.Context, cfg *config.Config, name, pluginDir string) error
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
	// Report lists the app-ready steps (nil for a dry run). Reconciled is true
	// once the Reconcile hook ran, so the caller must not reconcile again.
	Report     *appready.Report
	Reconciled bool
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

	rn := appready.NewRunner(b.Slug)
	if !opts.DryRun {
		defer func() { rep := rn.Report(); result.Report = &rep }()
	}

	// Resolve plugin version pins via T22 (the "fetch" step).
	fetchStart := time.Now()
	pins, err := resolvePlan(ctx, b, result, opts.Strict)
	if err != nil {
		rn.Record(appready.StepFetch, appready.ResultFailed, time.Since(fetchStart), err.Error())
		return result, err
	}
	planned := result.Planned
	rn.Record(appready.StepFetch, appready.ResultOK, time.Since(fetchStart), fmt.Sprintf("%d plugins resolved", len(planned)))

	// Print the plan.
	printPlan(out, b, result.Channel, planned, pins, result.Skipped, opts)

	if opts.DryRun {
		_, _ = fmt.Fprintln(out, "(dry-run) no changes made.")
		appready.PrintPlan(out, b.Slug, planned)
		return result, nil
	}

	// License pre-flight (see installer_licence.go). A free bundle makes no
	// ping_api bundle-entitlement call; the per-plugin check still runs, and for
	// it also honours the cached entitlement list (installer_helpers_licence.go).
	if opts.licenseChecker == nil {
		opts.licenseChecker = licenceChecker(b.IsFree())
	}
	if err := rn.Step(appready.StepLicence, func() (string, error) {
		if b.IsFree() {
			return "free bundle: no bundle entitlement call; plugin licences checked", checkLicence(ctx, b, planned, opts)
		}
		return "", checkLicence(ctx, b, planned, opts)
	}); err != nil {
		return result, err
	}

	pluginDir := opts.PluginDir
	if pluginDir == "" {
		pluginDir = defaultPluginDir()
	}
	store := newLedgerStore(pluginDir)
	// D-0322: hold the operation lock from the before-snapshot to the ledger step,
	// so a concurrent install, remove or ledger writer waits (with progress).
	err = store.Operation(ctx, "bundle install "+b.Slug, func() error {
		return installLocked(ctx, b, store, pluginDir, rn, planned, pins, opts, out, result)
	})
	return result, err
}

// installLocked is the part of Install that runs under the operation lock.
func installLocked(ctx context.Context, b Bundle, store *ledger.Store, pluginDir string, rn *appready.Runner,
	planned []string, pins VersionPins, opts InstallOpts, out io.Writer, result *InstallResult) error {
	// Bootstrap the ledger from the pre-install state before anything changes.
	if _, err := store.EnsureBootstrapped(); err != nil {
		rn.Record(appready.StepLedger, appready.ResultFailed, 0, err.Error())
		return fmt.Errorf("bundle %q: reading the install ledger before install: %w", b.Slug, err)
	}

	// What was on disk before this install: those members have no bundle origin.
	before := buildInstalledVersionMap(pluginDir)

	cfg := opts.Config
	if cfg == nil {
		loaded, lerr := loadConfigOrDefault()
		if lerr != nil {
			return fmt.Errorf("loading project config: %w", lerr)
		}
		cfg = loaded
	}
	mr := &memberRun{b: b, opts: opts, out: out, result: result, planned: planned, pins: pins, pluginDir: pluginDir, cfg: cfg,
		installFn: opts.installer, removeFn: opts.remover}
	if mr.installFn == nil {
		mr.installFn = func(ctx context.Context, cfg *config.Config, name, pd string) error {
			return plugin.Install(ctx, cfg, name, pd)
		}
	}
	if mr.removeFn == nil {
		mr.removeFn = func(ctx context.Context, cfg *config.Config, name, pd string) error {
			return plugin.Remove(ctx, cfg, name, pd, false, true)
		}
	}
	if err := rn.Step(appready.StepInstall, func() (string, error) {
		err := mr.run(ctx)
		return fmt.Sprintf("%d installed, %d unchanged", len(result.Installed), len(planned)-len(result.Installed)), err
	}); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(out, "\nBundle %q (%s) installed: %d plugins.\n", b.Name, b.Slug, len(result.Installed))
	if len(result.Skipped) > 0 {
		_, _ = fmt.Fprintf(out, "Skipped (missing from registry): %s\n", strings.Join(result.Skipped, ", "))
		// A bundle with a member missing is not the app the bundle describes (D-0372).
		rn.Unverified("bundle members skipped (not in the registry): " + strings.Join(result.Skipped, ", "))
	}
	return finishInstall(ctx, rn, store, pluginDir, planned, before, opts, result)
}

// resolvePlan resolves the version pins and sets result.Planned / Skipped. In
// non-strict mode a plugin missing from the registry is skipped with a warning.
func resolvePlan(ctx context.Context, b Bundle, result *InstallResult, strict bool) (VersionPins, error) {
	pins, err := ResolveBundleVersions(ctx, b.Slug, result.Channel)
	if err != nil {
		// Strict mode requires every plugin in the bundle to be in the registry.
		// Non-strict mode degrades to "install whatever IS in the registry".
		if strict {
			return nil, fmt.Errorf("resolve bundle versions (strict): %w", err)
		}
		// Try a per-plugin probe to determine which plugins are missing.
		partialPins, partialSkipped, probeErr := probePartialBundle(ctx, b, result.Channel)
		if probeErr != nil {
			return nil, fmt.Errorf("resolve bundle versions: %w", err)
		}
		pins = partialPins
		result.Skipped = partialSkipped
		if len(pins) == 0 {
			return nil, fmt.Errorf("no plugins in bundle %q are eligible under channel %q", b.Slug, result.Channel)
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
	return pins, nil
}
