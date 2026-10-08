package plugin

// tier_resolve.go — install-time resolution for a plugin slug served more
// than once by the registry.
//
// Purpose: OWNER-ACTIONS.md item 15 (2026-09-04, decision (b)) found that
// `nself plugin install cron` silently installed the FREE plugin even for a
// customer entitled to the pro tier, because findPlugin/GetPlugin return the
// first registry match and the served registry lists cron and notify twice
// (free then pro). This file replaces that first-match behaviour for any
// name with more than one registry entry: a genuine free/pro pair of the
// same product (TierPair: true on BOTH entries) resolves by license
// entitlement; any other same-slug collision is refused as a registry-data
// error rather than silently picking one.
// Inputs: the fetched Registry, the requested plugin name, an optional
// explicit --tier override ("", "free", or "pro"), and an EntitlementFunc
// (defaultEntitlement in production, a fixture in tests).
// Outputs: the single PluginManifest to install, or errs.ErrDuplicatePluginSlug
// / errs.ErrTierNotEntitled / errs.ErrPluginNotFound.
// Constraints: never falls back to first-match on an unresolved ambiguity —
// that is the exact bug this file exists to close.

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
)

// EntitlementFunc reports whether the operator's current license entitles at
// least one of the given bundles.nself.org slugs. Production callers use
// defaultEntitlement; tests inject a fixture so resolution logic is
// verifiable without a live ping_api call.
type EntitlementFunc func(ctx context.Context, bundles []string) (bool, error)

// defaultEntitlement checks bundle-level entitlement via the same
// license.BundleEntitled call the bundle installer uses (bundle/installer.go
// Phase 1), reusing the operator's already-configured license key. A plugin
// entry with no Bundles (a standalone pro plugin, not part of a bundle) is
// never entitled by this path — install falls through to the ordinary
// per-plugin checkLicense flow for that case.
func defaultEntitlement(ctx context.Context, bundles []string) (bool, error) {
	if len(bundles) == 0 {
		return false, nil
	}
	key := license.CollectLicenseKey()
	if key == "" {
		return false, nil
	}
	var lastErr error
	for _, b := range bundles {
		ok, err := license.BundleEntitled(ctx, key, b)
		if ok {
			return true, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	return false, lastErr
}

// findPluginEntries returns every registry entry whose name case-insensitively
// matches name, in registry order. Unlike findPlugin/GetPlugin this does not
// stop at the first match — it is the primitive tier resolution is built on.
func findPluginEntries(reg *Registry, name string) []*PluginManifest {
	var out []*PluginManifest
	for i := range reg.Plugins {
		if strings.EqualFold(reg.Plugins[i].Name, name) {
			out = append(out, &reg.Plugins[i])
		}
	}
	return out
}

// splitTierPair separates a two-entry match into its free and pro manifests.
// Returns ok=false — a registry-data error, not a resolvable ambiguity —
// unless BOTH entries declare TierPair: true and they occupy the two
// different tiers exactly once each (one free, one pro).
func splitTierPair(entries []*PluginManifest) (free, pro *PluginManifest, ok bool) {
	if len(entries) != 2 {
		return nil, nil, false
	}
	a, b := entries[0], entries[1]
	if !a.TierPair || !b.TierPair {
		return nil, nil, false
	}
	switch {
	case a.Tier == "free" && b.Tier == "pro":
		return a, b, true
	case a.Tier == "pro" && b.Tier == "free":
		return b, a, true
	default:
		return nil, nil, false
	}
}

// duplicateSlugError formats the hard-error report for a same-slug collision
// that is NOT a declared tier pair — every entry is named so the operator (or
// the registry maintainer) can see exactly what collided, per OWNER-ACTIONS.md
// item 15's "never silent first-match" rule.
func duplicateSlugError(name string, entries []*PluginManifest) error {
	descs := make([]string, len(entries))
	for i, e := range entries {
		descs[i] = fmt.Sprintf("%s@%s (license=%s)", e.Name, e.Version, LicenseValue(e.Tier))
	}
	return fmt.Errorf("%w: %q resolves to %d registry entries that are not a declared tier_pair: %s",
		errs.ErrDuplicatePluginSlug, name, len(entries), strings.Join(descs, ", "))
}

// ResolvePlugin picks the single registry entry `nself plugin install <name>`
// (or a bundle's own install loop) should install.
//
//   - Zero matches: errs.ErrPluginNotFound.
//   - One match: returned as-is; tierOverride is validated against its own
//     tier and rejected if it disagrees (e.g. --tier pro for a plugin that
//     only has a free entry).
//   - Two matches that are a declared tier pair (TierPair: true on both):
//     tierOverride "free"/"pro" picks that entry directly ("pro" still runs
//     the entitlement check below — an override never bypasses licensing);
//     no override resolves by entitlement, defaulting to free when the
//     entitlement check is false, erroring, or the operator has no key.
//   - Two-or-more matches that are NOT a declared tier pair: duplicateSlugError
//     — a registry-data problem, never resolved by picking one.
func ResolvePlugin(ctx context.Context, reg *Registry, name, tierOverride string, entitled EntitlementFunc) (*PluginManifest, error) {
	if entitled == nil {
		entitled = defaultEntitlement
	}
	tierOverride = strings.ToLower(strings.TrimSpace(tierOverride))
	if tierOverride == LicenseLicensed {
		tierOverride = WireTierPro
	}
	if tierOverride != "" && tierOverride != "free" && tierOverride != "pro" {
		return nil, fmt.Errorf("invalid --tier %q: must be \"free\" or \"licensed\"", tierOverride)
	}

	entries := findPluginEntries(reg, name)
	switch len(entries) {
	case 0:
		return nil, errs.ErrPluginNotFound
	case 1:
		only := entries[0]
		if tierOverride != "" && only.Tier != "" && only.Tier != tierOverride {
			return nil, fmt.Errorf("plugin %q only has a %q entry in the registry, not %q", name, only.Tier, tierOverride)
		}
		return only, nil
	}

	free, pro, ok := splitTierPair(entries)
	if !ok {
		return nil, duplicateSlugError(name, entries)
	}

	switch tierOverride {
	case "free":
		return free, nil
	case "pro":
		ok, err := entitled(ctx, pro.Bundles)
		if err != nil {
			return nil, fmt.Errorf("checking entitlement for %q Licensed plugin: %w", name, err)
		}
		if !ok {
			return nil, notEntitledError(name, pro)
		}
		return pro, nil
	default:
		ok, err := entitled(ctx, pro.Bundles)
		if err != nil || !ok {
			// Fail closed to free, never to an error: the operator asked for
			// "cron", not specifically the pro tier, and free always installs.
			return free, nil
		}
		return pro, nil
	}
}

// notEntitledError names the bundle the operator needs, matching the message
// shape bundle/installer.go's Phase 1 check already uses.
func notEntitledError(name string, pro *PluginManifest) error {
	if len(pro.Bundles) == 0 {
		return fmt.Errorf("%w: plugin %q (Licensed plugin requires a plugin-level license, run 'nself license set <key>')", errs.ErrTierNotEntitled, name)
	}
	return fmt.Errorf("%w: plugin %q requires %s (or ɳSelf+) — buy at https://nself.org/pricing or run 'nself license set <key>'",
		errs.ErrTierNotEntitled, name, BundleLabel(strings.Join(pro.Bundles, "/")))
}
