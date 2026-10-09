package commands

// Purpose: Core `nself plugin install` handler split out of plugin.go
// (CLI-R12 Batch B mechanical file-size split), plus its --dry-run helper.
// Holds the official-vs-third-party arg split, SBOM/license-skip flag
// handling, and the actual per-plugin install loop.
// Inputs: cobra command flags (--key, --force, --allow-eol,
// --skip-sbom-check, --preview, --dry-run, --with-optional, --show-graph,
// --yes, --checksum) and positional plugin name/URL args.
// Outputs: stderr progress messages and telemetry side effects; errors wrap
// per-plugin install failures.
// Constraints: pure move, no behavior change. Delegates to
// runPluginInstallPreview/runPluginInstallShowGraph (plugin_install_graph.go)
// and maybeRegisterFreeAccount/maybeShowFreeUpsell (plugin_install_upsell.go).

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/reconcile"

	"github.com/spf13/cobra"
)

func runPluginInstall(cmd *cobra.Command, args []string) error {
	key, _ := cmd.Flags().GetString("key")
	force, _ := cmd.Flags().GetBool("force")
	allowEOL, _ := cmd.Flags().GetBool("allow-eol")       // S58-T03
	skipSBOM, _ := cmd.Flags().GetBool("skip-sbom-check") // S2.T12
	preview, _ := cmd.Flags().GetBool("preview")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	withOptional, _ := cmd.Flags().GetBool("with-optional")
	showGraph, _ := cmd.Flags().GetBool("show-graph")
	yes, _ := cmd.Flags().GetBool("yes")
	checksum, _ := cmd.Flags().GetString("checksum")
	tier, _ := cmd.Flags().GetString("tier")

	// --tier overrides entitlement-based resolution for a slug served as both
	// free and pro (OWNER-ACTIONS.md item 15). Threaded via env var, matching
	// the existing NSELF_SKIP_SBOM_CHECK / NSELF_PLUGIN_LICENSE_KEY convention
	// so plugin.installLocked (and its dependency-install recursion) sees it
	// without widening plugin.Install's signature.
	tier = strings.ToLower(strings.TrimSpace(tier))
	if tier == plugin.LicenseLicensed {
		tier = plugin.WireTierPro
	}
	if tier != "" && tier != "free" && tier != plugin.WireTierPro {
		return fmt.Errorf(`invalid --tier %q: must be "free" or "licensed" ("pro" remains an alias)`, tier)
	}
	if tier != "" {
		if err := os.Setenv("NSELF_PLUGIN_INSTALL_TIER", tier); err != nil {
			return fmt.Errorf("setting tier override: %w", err)
		}
	}

	// CLI-R16: "official by name, third-party by URL" — split the requested
	// refs up front so registry-only flows (preview/show-graph/dry-run, the
	// free-account flow, license checks) only ever see official names.
	var officialNames, thirdPartyURLs []string
	for _, a := range args {
		if plugin.IsThirdPartyInstallSource(a) {
			thirdPartyURLs = append(thirdPartyURLs, a)
		} else {
			officialNames = append(officialNames, a)
		}
	}
	if len(thirdPartyURLs) > 0 && (preview || dryRun || showGraph) {
		return fmt.Errorf("--preview, --dry-run, and --show-graph require registry-resolved dependency data and are not supported for third-party URL installs")
	}
	if checksum != "" && len(thirdPartyURLs) != 1 {
		return fmt.Errorf("--checksum applies to exactly one third-party URL install at a time (got %d); install it separately", len(thirdPartyURLs))
	}

	// S2.T12: --skip-sbom-check sets the env var read by plugin.installLocked.
	// Air-gapped installs only — emit a prominent warning when used.
	if skipSBOM {
		fmt.Fprintf(os.Stderr, "WARNING: SBOM verification disabled (--skip-sbom-check). For air-gapped installs only.\n")
		if err := os.Setenv("NSELF_SKIP_SBOM_CHECK", "1"); err != nil {
			return fmt.Errorf("setting NSELF_SKIP_SBOM_CHECK: %w", err)
		}
	}

	// Security gate: NSELF_LICENSE_SKIP_VERIFY=1 requires --force as explicit acknowledgment.
	// Standalone skip (without --force) is rejected to prevent accidental bypass in scripts.
	if os.Getenv("NSELF_LICENSE_SKIP_VERIFY") == "1" && !force {
		return fmt.Errorf("NSELF_LICENSE_SKIP_VERIFY requires --force flag; standalone skip is not permitted")
	}
	if os.Getenv("NSELF_LICENSE_SKIP_VERIFY") == "1" && force {
		fmt.Fprintf(os.Stderr, "warning: license verification bypassed via NSELF_LICENSE_SKIP_VERIFY (--force acknowledged)\n")
	}

	// If a license key is provided via flag, set it in the environment
	// so the plugin manager's license check picks it up.
	if key != "" {
		if err := os.Setenv("NSELF_PLUGIN_LICENSE_KEY", key); err != nil {
			return fmt.Errorf("setting license key: %w", err)
		}
	}

	ctx := context.Background()
	registryURL := os.Getenv("NSELF_PLUGIN_REGISTRY")
	// Third-party-only installs never touch the official registry, so skip
	// the reachability check when there's nothing official to resolve.
	if len(officialNames) > 0 {
		if err := plugin.ValidateNetworkAccess(ctx, registryURL); err != nil {
			return err
		}
	}

	// --preview: resolve and print the dependency tree, then exit without installing.
	if preview {
		return runPluginInstallPreview(ctx, officialNames, registryURL, withOptional)
	}

	// --show-graph: resolve deps, detect cycles, print topo-sorted DAG, exit without installing.
	if showGraph {
		return runPluginInstallShowGraph(ctx, officialNames, registryURL)
	}

	// --dry-run: show what would be installed and checkpoint without making changes.
	if dryRun {
		return runPluginInstallDryRun(ctx, officialNames, registryURL)
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// S20: Free-plugin account flow.
	// If no license key is set and all requested plugins are free, prompt the
	// user to create a nself_free_ account for rate-limiting + upsell tracking.
	// Skip when NSELF_LICENSE_SKIP_VERIFY=1 (offline/CI) or when a key already exists.
	pingURL := os.Getenv("NSELF_PING_URL")
	if pingURL == "" {
		pingURL = "https://ping.nself.org"
	}
	existingKey := os.Getenv("NSELF_PLUGIN_LICENSE_KEY")
	if len(officialNames) > 0 && existingKey == "" && os.Getenv("NSELF_LICENSE_SKIP_VERIFY") != "1" {
		if err := maybeRegisterFreeAccount(ctx, pingURL, officialNames); err != nil {
			// Non-fatal: free account creation failure never blocks plugin install.
			fmt.Fprintf(os.Stderr, "note: could not create free account (%v); continuing without one\n", err)
		}
	}

	pluginDir := resolvePluginDir()

	// Install each named plugin. Collect per-plugin errors so that a failure on
	// one plugin does not abort the remaining installs.
	var failures []string
	var installed []string
	installedCount := 0
	for _, name := range officialNames {
		// S58-T03: EOL gate — check status before attempting install.
		if eolErr := plugin.CheckEOLBlock(ctx, name, allowEOL); eolErr != nil {
			fmt.Fprintf(os.Stderr, "  %v\n", eolErr)
			failures = append(failures, name)
			continue
		}

		fmt.Fprintf(os.Stderr, "Installing plugin %q...\n", name)
		if err := plugin.Install(ctx, cfg, name, pluginDir); err != nil {
			fmt.Fprintf(os.Stderr, "  error installing %q: %v\n", name, err)
			failures = append(failures, name)
			continue
		}
		fmt.Fprintf(os.Stderr, "Plugin %q installed successfully.\n", name)
		installedCount++
		installed = append(installed, name)
		// compat.V15(P7-LIVE-06): post-install build hint -> automatic reconcile
		if !compat.V15() {
			printPluginPostInstallHint(name)
		}

		// S20: Fire install telemetry for free-tier keys (non-blocking).
		currentKey := os.Getenv("NSELF_PLUGIN_LICENSE_KEY")
		if plugin.IsFreeKey(currentKey) {
			plugin.SendFreeInstallTelemetry(pingURL, currentKey, name)
		}
	}

	// CLI-R16: third-party URL installs — never touch the registry, no
	// license/EOL/telemetry handling (none of that is meaningful without a
	// registry entry). Each one requires interactive confirmation naming the
	// source host first, unless --yes was passed.
	for _, srcURL := range thirdPartyURLs {
		if err := confirmThirdPartyInstall(cmd, srcURL, yes); err != nil {
			fmt.Fprintf(os.Stderr, "  %v\n", err)
			failures = append(failures, srcURL)
			continue
		}
		fmt.Fprintf(os.Stderr, "Installing plugin from %s...\n", srcURL)
		if err := plugin.InstallFromURL(ctx, cfg, srcURL, pluginDir, checksum); err != nil {
			fmt.Fprintf(os.Stderr, "  error installing from %q: %v\n", srcURL, err)
			failures = append(failures, srcURL)
			continue
		}
		installedCount++
		installed = append(installed, srcURL)
	}

	// S20: Upsell prompt after 3rd successful free-plugin install.
	if installedCount > 0 {
		maybeShowFreeUpsell(installedCount)
	}
	// compat.V15(P7-LIVE-06): plugin install leaves generated state for a later build -> reconcile now
	return finishPluginInstall(compat.V15(), installed, failures, func() error {
		return reconcileAfterExtension(cmd, reconcile.Trigger{Kind: reconcile.TriggerPlugin, Subject: strings.Join(installed, ",")})
	})
}

// finishPluginInstall reconciles once after any successful install (v1.5) and
// reports every failure together: a failed reconcile never hides the plugins
// that failed to install, and vice versa (errors.Join, as bundle install does).
func finishPluginInstall(v15 bool, installed, failures []string, reconcileFn func() error) error {
	var all []error
	if v15 && len(installed) > 0 {
		if err := reconcileFn(); err != nil {
			all = append(all, err)
		}
	}
	if len(failures) > 0 {
		all = append(all, fmt.Errorf("failed to install: %s", strings.Join(failures, ", ")))
	}
	return errors.Join(all...)
}

// runPluginInstallDryRun simulates bundle installation and shows what would change
// without making any modifications. Resolves deps, checks licenses, reports changes.
func runPluginInstallDryRun(ctx context.Context, pluginNames []string, registryURL string) error {
	fmt.Fprintf(os.Stderr, "[DRY RUN] Simulating bundle install for: %s\n", strings.Join(pluginNames, ", "))

	cacheDir := plugin.DefaultCacheDir()
	reg, err := plugin.FetchRegistry(ctx, registryURL, cacheDir)
	if err != nil {
		return fmt.Errorf("fetching registry: %w", err)
	}

	var toInstall []string
	seen := make(map[string]bool)

	// Collect all plugins + dependencies
	var collect func(string) error
	collect = func(name string) error {
		if seen[name] {
			return nil
		}
		seen[name] = true

		m, found := plugin.FindPluginByName(reg, name)
		if !found {
			return fmt.Errorf("plugin %q not found", name)
		}

		// Recurse on dependencies
		for _, dep := range m.Dependencies {
			if err := collect(dep); err != nil {
				return err
			}
		}

		toInstall = append(toInstall, name)
		return nil
	}

	for _, name := range pluginNames {
		if err := collect(name); err != nil {
			return err
		}
	}

	fmt.Fprintf(os.Stderr, "[DRY RUN] Would install %d plugin(s):\n", len(toInstall))
	for _, name := range toInstall {
		m, _ := plugin.FindPluginByName(reg, name)
		fmt.Fprintf(os.Stderr, "  • %s v%s\n", name, m.Version)
	}
	return nil
}
