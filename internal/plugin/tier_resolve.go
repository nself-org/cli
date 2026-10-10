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
// P7-PLUG-17 (v1.5 only): an installed plugin keeps its tier on update; E131
// when it cannot. InstalledTier reports it for `plugin info --json`.
// Constraints: never falls back to first-match on an unresolved ambiguity —
// that is the exact bug this file exists to close.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/compat"
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

// Tier reasons reported with a resolution (and by `plugin info --json`).
const (
	TierReasonInstalled   = "installed"    // an installed plugin keeps its tier
	TierReasonOverride    = "override"     // --tier chose it
	TierReasonEntitlement = "entitlement"  // the licence entitles the Licensed tier
	TierReasonDefaultFree = "default-free" // nothing else decided, so free
)

// tierHint is what Update tells the resolver about the plugin it is updating.
// It travels in the context (not the process environment), is keyed by plugin
// name so dependency installs are unaffected, and dies with the call.
type tierHint struct{ name, installed, override string }

type tierHintKey struct{}

// withTierHint returns ctx carrying the installed tier (and an optional
// --tier override) of the plugin named name.
func withTierHint(ctx context.Context, name, installed, override string) context.Context {
	return context.WithValue(ctx, tierHintKey{}, tierHint{name, installed, override})
}

// tierClass reduces a registry tier to "free", "pro" (any licensed tier) or "".
func tierClass(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "":
		return ""
	case WireTierFree:
		return WireTierFree
	default:
		return WireTierPro
	}
}

// ResolvePlugin picks the single registry entry `nself plugin install <name>`
// (or a bundle's own install loop) should install; see ResolvePluginTier.
func ResolvePlugin(ctx context.Context, reg *Registry, name, tierOverride string, entitled EntitlementFunc) (*PluginManifest, error) {
	installed := ""
	if h, ok := ctx.Value(tierHintKey{}).(tierHint); ok && strings.EqualFold(h.name, name) {
		installed = h.installed
		if h.override != "" {
			tierOverride = h.override
		}
	}
	m, _, err := ResolvePluginTier(ctx, reg, name, tierOverride, installed, entitled)
	return m, err
}

// ResolvePluginTier resolves like ResolvePlugin and also returns the reason.
// Zero matches: ErrPluginNotFound. One match: returned as-is (a --tier that
// disagrees with it is rejected). A declared tier pair: --tier picks the
// entry ("pro" still needs the entitlement); otherwise entitlement decides
// and falls back to free. Any other duplicate: duplicateSlugError, never a
// first-match pick.
//
// installedTier is the tier of the copy being updated ("" for a fresh
// install). In v1.5 mode it wins over entitlement unless tierOverride is
// set, and a plugin that cannot keep it fails with E131 instead of switching.
func ResolvePluginTier(ctx context.Context, reg *Registry, name, tierOverride, installedTier string, entitled EntitlementFunc) (*PluginManifest, string, error) {
	if entitled == nil {
		entitled = defaultEntitlement
	}
	tierOverride = strings.ToLower(strings.TrimSpace(tierOverride))
	if tierOverride == LicenseLicensed {
		tierOverride = WireTierPro
	}
	if tierOverride != "" && tierOverride != "free" && tierOverride != "pro" {
		return nil, "", fmt.Errorf("invalid --tier %q: must be \"free\" or \"licensed\"", tierOverride)
	}
	// compat.V15(P7-PLUG-17): an update re-resolves a tier pair by entitlement and can switch the installed tier -> the installed tier sticks unless --tier is given
	sticky := compat.V15() && tierOverride == "" && tierClass(installedTier) != ""

	entries := findPluginEntries(reg, name)
	switch len(entries) {
	case 0:
		return nil, "", errs.ErrPluginNotFound
	case 1:
		only := entries[0]
		if tierOverride != "" && only.Tier != "" && only.Tier != tierOverride {
			return nil, "", fmt.Errorf("plugin %q only has a %q entry in the registry, not %q", name, only.Tier, tierOverride)
		}
		switch {
		case tierOverride != "":
			return only, TierReasonOverride, nil
		case sticky && only.Tier != "" && tierClass(only.Tier) != tierClass(installedTier):
			return nil, "", tierChangeError(name, installedTier, only.Tier, nil)
		case sticky:
			return only, TierReasonInstalled, nil
		case tierClass(only.Tier) == WireTierPro:
			return only, TierReasonEntitlement, nil
		}
		return only, TierReasonDefaultFree, nil
	}

	free, pro, ok := splitTierPair(entries)
	if !ok {
		return nil, "", duplicateSlugError(name, entries)
	}

	want, reason := tierOverride, TierReasonOverride
	if sticky {
		want, reason = tierClass(installedTier), TierReasonInstalled
	}
	switch want {
	case "free":
		return free, reason, nil
	case "pro":
		ok, err := entitled(ctx, pro.Bundles)
		if err != nil {
			return nil, "", fmt.Errorf("checking entitlement for %q Licensed plugin: %w", name, err)
		}
		if !ok && sticky {
			return nil, "", tierChangeError(name, installedTier, WireTierFree, notEntitledError(name, pro))
		}
		if !ok {
			return nil, "", notEntitledError(name, pro)
		}
		return pro, reason, nil
	default:
		ok, err := entitled(ctx, pro.Bundles)
		if err != nil || !ok {
			// Fail closed to free, never to an error: the operator asked for
			// "cron", not specifically the pro tier, and free always installs.
			return free, TierReasonDefaultFree, nil
		}
		return pro, TierReasonEntitlement, nil
	}
}

// tierChangeError is E131: an update would move an installed plugin to
// another tier. cause (may be nil) is what forces the move.
func tierChangeError(name, from, to string, cause error) error {
	e := errs.Newf("E131", "plugin %q is installed as %s; this update would make it %s (pass --tier %s to switch on purpose)",
		name, LicenseValue(tierClass(from)), LicenseValue(tierClass(to)), LicenseValue(tierClass(to)))
	e.Wrapped = cause
	return e
}

// InstalledTier reports the tier an installed plugin keeps (licence
// vocabulary: free or licensed) and why, for `plugin info --json`. ok is
// false when the plugin is not installed or its manifest names no tier.
func InstalledTier(pluginDir, name string) (tier, reason string, ok bool) {
	raw := installedTierWire(filepath.Join(pluginDir, name))
	if tierClass(raw) == "" {
		return "", "", false
	}
	return LicenseValue(tierClass(raw)), TierReasonInstalled, true
}

// installedTierWire reads only the tier key of an installed plugin.json, so a
// manifest that fails full validation still keeps its tier (v1 and v2 files
// both carry it).
func installedTierWire(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		return ""
	}
	var m struct {
		Tier string `json:"tier"`
	}
	if json.Unmarshal(data, &m) != nil {
		return ""
	}
	return m.Tier
}

// notEntitledError names the bundle the operator needs, matching the message
// shape bundle/installer.go's Phase 1 check already uses.
func notEntitledError(name string, pro *PluginManifest) error {
	if len(pro.Bundles) == 0 {
		return fmt.Errorf("%w: plugin %q (Licensed plugin requires a plugin-level license, run 'nself license set <key>')", errs.ErrTierNotEntitled, name)
	}
	requirement := fmt.Sprintf("a licence for the %s Bundle", pro.Bundles[0])
	if len(pro.Bundles) > 1 {
		requirement = fmt.Sprintf("a licence for one of these Bundles: %s", strings.Join(pro.Bundles, ", "))
	}
	return fmt.Errorf("%w: plugin %q requires %s (or ɳSelf+) — buy at https://nself.org/pricing or run 'nself license set <key>'",
		errs.ErrTierNotEntitled, name, requirement)
}
