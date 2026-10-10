package commands

// Purpose: resolve `nself license show --json`'s bundles_unlocked value from
// the shared internal/bundle resolver (bundles.json / ADR-P6-03). Split out
// of license_status.go for file size (engineering-standard <=300 lines, ASI
// Policy 3).
// Inputs: a license.ProductPrefix (from license.DetectProduct) and a
// context for the bundle.EnsureLoaded fetch.
// Outputs: the ordered list of unlocked bundle display names.
// Constraints: never fabricates a union — degrades to the raw product
// display name when bundles.json cannot be loaded or the product code has
// no known bundle mapping. See resolveBundlesUnlocked's doc comment.

import (
	"context"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/license"
)

// productToBundleSlug maps a license key's detected product code
// (license.ProductPrefix.Product) to its bundles.json canonical bundle
// slug, for keys tied to exactly one paid bundle. Products with no 1:1
// bundle mapping — legacy business tiers "pro"/"enterprise"/"max", or a
// product code with no bundles.json counterpart yet — are intentionally
// absent; resolveBundlesUnlocked falls back to the raw product display
// name for those rather than guessing a bundle slug.
var productToBundleSlug = map[string]string{
	"claw":   "claw",
	"clawde": "clawde",
	"chat":   "chat",
	"family": "family",
	"media":  "tv", // nself_media_ prefix predates bundles.json's "tv" slug rename
}

// resolveBundlesUnlocked computes `nself license show --json`'s
// bundles_unlocked value: the union of every paid bundle a license key
// entitles, resolved from bundles.json (ADR-P6-03) via the shared
// internal/bundle resolver — never a single hand-picked product name.
//
// P6-E6-W4-S3-T7 (2026-09-03) found this previously always resolved to
// []string{pp.DisplayName} (e.g. just "ɳSelf+"), regardless of tier, because
// no code path called the bundle union resolver at all. Rules, in order:
//
//   - owner/plus keys unlock every currently-installable paid bundle in
//     bundles.json (IsInstallable excludes the synthetic "nself-plus"
//     meta-bundle — Ruling 2's one hand-coded membership rule — and IsFree
//     excludes the free "task" bundle, installable since P7-PLUG-19).
//   - a key tied to exactly one bundle (productToBundleSlug) unlocks that
//     one bundle.
//   - any other/unrecognized product code, or a bundle slug not present in
//     the currently loaded bundles.json, degrades to the raw product
//     display name (the pre-fix behavior) rather than fabricating a bundle
//     list.
//   - if bundles.json cannot be loaded at all (network failure with no
//     local cache — the production plugins.nself.org/bundles.json endpoint
//     currently 404s, tracked separately, T8 CF worker owner), this also
//     degrades to the raw product display name: honest partial
//     information, never a fabricated union.
func resolveBundlesUnlocked(ctx context.Context, pp *license.ProductPrefix) []string {
	if pp == nil {
		return nil
	}
	if err := bundle.EnsureLoaded(ctx); err != nil {
		return []string{pp.DisplayName}
	}
	if pp.Product == "plus" || pp.Product == "owner" {
		var names []string
		for _, b := range bundle.All() {
			if !b.IsInstallable() || b.IsFree() {
				continue
			}
			names = append(names, b.Name)
		}
		if len(names) == 0 {
			return []string{pp.DisplayName}
		}
		return names
	}
	if slug, ok := productToBundleSlug[pp.Product]; ok {
		if b, found := bundle.Get(slug); found {
			return []string{b.Name}
		}
	}
	return []string{pp.DisplayName}
}
