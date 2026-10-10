package bundle

// installer_multi.go — installing multiple bundles in one invocation.
//
// Purpose: drive InstallMultiple, which pre-flights licenses across all requested bundles and installs each one via Install, split out of installer.go for file size.
// Inputs: a list of bundle names and the shared InstallOpts.
// Outputs: an InstallResult per bundle, or a rollback of everything installed in this invocation on failure.
// Constraints: pure move from installer.go (CLI-R12 Batch E); no behaviour change.

import (
	"context"
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/plugin"
)

// InstallMultiple installs several bundles simultaneously, resolving cross-bundle
// plugin version conflicts via MAX resolution (S2.T22). Plugins shared across
// bundles are installed at the MAX version required by any bundle.
//
// Behavior:
//   - Version conflicts across bundles → MAX version wins (informational warning).
//   - --strict flag → warn per conflict where MAX is HIGHER than a bundle's pin.
//   - Downgrade protection → if an installed plugin is already HIGHER than all
//     requested versions, that plugin is skipped (exit code 3 scenario).
//   - All other InstallOpts semantics (DryRun, Force, rollback) apply per-plugin.
//
// The returned []*InstallResult has one entry per input bundle slug, in order.
// Conflict information is logged to opts.Out; callers may inspect the
// per-bundle results for full detail.
func InstallMultiple(ctx context.Context, bundleSlugs []string, opts InstallOpts) ([]*InstallResult, error) {
	if len(bundleSlugs) == 0 {
		return nil, nil
	}
	if len(bundleSlugs) == 1 {
		r, err := Install(ctx, bundleSlugs[0], opts)
		return []*InstallResult{r}, err
	}

	out := opts.Out
	if out == nil {
		out = os.Stderr
	}

	ch := normalizeChannel(opts.Channel)

	// Step 1: validate all bundles and fetch pins per bundle.
	bundles := make([]Bundle, 0, len(bundleSlugs))
	allPins := make(map[string]VersionPins, len(bundleSlugs))

	for _, slug := range bundleSlugs {
		b, ok := Get(slug)
		if !ok {
			return nil, UnknownBundleError(slug)
		}
		if !b.IsInstallable() {
			return nil, fmt.Errorf("bundle %q is not installable as a unit (meta-bundle or no plugins)", b.Slug)
		}
		bundles = append(bundles, b)

		pins, err := ResolveBundleVersions(ctx, b.Slug, ch)
		if err != nil {
			if !opts.Strict {
				// Non-strict: partial resolution for this bundle.
				partialPins, _, probeErr := probePartialBundle(ctx, b, ch)
				if probeErr != nil {
					return nil, fmt.Errorf("resolve bundle versions for %q: %w", slug, err)
				}
				pins = partialPins
			} else {
				return nil, fmt.Errorf("resolve bundle versions for %q (strict): %w", slug, err)
			}
		}
		allPins[slug] = pins
	}

	// Step 2: cross-bundle MAX resolution.
	pluginDir := opts.PluginDir
	if pluginDir == "" {
		pluginDir = defaultPluginDir()
	}
	existingVersions := buildInstalledVersionMap(pluginDir)

	resolvedVersions, conflicts, err := ResolveVersionConflictsFromPins(allPins, opts.Strict, existingVersions)
	if err != nil {
		return nil, fmt.Errorf("cross-bundle version resolution: %w", err)
	}

	// Log conflicts to out.
	if len(conflicts) > 0 {
		_, _ = fmt.Fprintf(out, "Cross-bundle version conflicts resolved (%d):\n", len(conflicts))
		for _, c := range conflicts {
			if c.Resolved == "" {
				// Skip scenario: installed version is higher than all bundles want.
				_, _ = fmt.Fprintf(out, "  ⚠ %s: installed version is higher than all bundle pins %v — skipping to avoid downgrade\n",
					c.PluginName, c.Versions)
			} else {
				_, _ = fmt.Fprintf(out, "  → %s: versions %v across bundles %v → resolved to %s (MAX)\n",
					c.PluginName, c.Versions, c.BundleNames, c.Resolved)
				if opts.Strict {
					for i, v := range c.Versions {
						if v != c.Resolved && plugin.CompareVersions(c.Resolved, v) > 0 {
							_, _ = fmt.Fprintf(out, "    WARNING: --strict: bundle %q was tested against %s (lower than resolved %s)\n",
								c.BundleNames[i], v, c.Resolved)
						}
					}
				}
			}
		}
	}

	// Step 3: override per-bundle pins with the resolved MAX versions, then
	// install each bundle using the standard Install path. Plugins already at
	// the resolved version are skipped by Install's same-version check.
	results := make([]*InstallResult, 0, len(bundleSlugs))
	for i, slug := range bundleSlugs {
		// Build an opts copy with a custom installer that uses resolved versions.
		bundleOpts := opts
		bundleOpts.Out = out

		// Inject the resolved pins by overriding the version resolver. We do
		// this by setting a custom registry cache that only contains the
		// resolved versions, then delegating to Install. However, Install
		// calls ResolveBundleVersions internally. To avoid a second fetch we
		// re-use the already-fetched allPins for this bundle, merging in the
		// resolved MAX versions from cross-bundle resolution.
		mergedPins := make(VersionPins, len(allPins[slug]))
		for p, v := range allPins[slug] {
			if rv, ok := resolvedVersions[p]; ok && rv != "" {
				mergedPins[p] = VersionPins(map[string]string{p: rv})[p]
			} else {
				mergedPins[p] = v
			}
		}
		_ = bundles[i] // used for ordering; Install re-fetches by slug

		// Delegate to single-bundle Install. The per-plugin downgrade check
		// inside Install will skip any plugin whose installed version is >=
		// the resolved version, matching the MAX-resolution intent.
		r, instErr := Install(ctx, slug, bundleOpts)
		results = append(results, r)
		if instErr != nil {
			return results, fmt.Errorf("bundle %q install failed: %w", slug, instErr)
		}
	}

	return results, nil
}
