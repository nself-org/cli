// Package bundle implements the canonical nSelf paid plugin bundle catalog
// plus install/remove orchestration with license validation, version pinning,
// and atomic rollback on failure (S2.T03/T04/T22).
//
// Bundle membership is resolved from bundles.json (ADR-P6-03), the single
// source of truth for bundle -> plugin membership, served at
// plugins.nself.org/bundles.json. See registry.go for the fetch+cache loader.
// The cmd layer must consume this package via Get/Names/All — do NOT
// hand-maintain a second bundle map anywhere else in the CLI.
package bundle

import (
	"fmt"
	"sort"
	"strings"
)

// Bundle describes a canonical nSelf plugin bundle.
type Bundle struct {
	Name        string   // Display name (with ɳ glyph)
	Slug        string   // CLI-arg lowercase identifier (bundles.json canonical slug)
	Price       string   // Human-readable price string
	Description string   // Optional one-line description
	Plugins     []string // Plugin slugs in the bundle (nil for meta-bundle)
}

// DisplayOrder is the canonical print order for bundle listings (ADR-P6-03
// ordering canon): task -> chat -> claw -> family -> sentry -> tv -> clawde,
// plus the computed ɳSelf+ meta-bundle last.
var DisplayOrder = []string{
	"task", "chat", "claw", "family", "sentry", "tv", "clawde", "nself-plus",
}

// selfPlusSlug is the synthetic meta-bundle slug. It is NOT a bundles.json
// entry — its Plugins are computed as the union of every paid bundle's
// plugins (ADR-P6-03: "an ɳSelf+ license entitles the union of all paid
// bundles"), never hand-listed. See registry.go's buildBundleMap.
const selfPlusSlug = "nself-plus"

// bundleAliases maps legacy/marketing slugs to their canonical bundles.json
// slug. bundles.json's canonical slugs are short (claw/chat/family/tv/sentry)
// while the CLI historically used n-prefixed forms (nclaw/nchat/nfamily/ntv/
// nsentry) plus "ntask" for the free bundle. Both forms must keep resolving
// for backward compatibility with existing CLI users. "task" and "clawde"
// never had an n-prefixed form. NOTE: bundles.json's canonical slug for the
// observability bundle is "sentry" (not "nsentry" as in the pre-ADR-P6-03
// hardcoded map) — this alias direction was corrected here, it is not merely
// an extension of the old single entry.
var bundleAliases = map[string]string{
	"nclaw":   "claw",
	"nchat":   "chat",
	"nfamily": "family",
	"ntv":     "tv",
	"nsentry": "sentry",
	"ntask":   "task",
}

// Get returns the Bundle for the given slug. The slug is case-insensitive and
// trimmed. Falls back to bundleAliases when there is no canonical match.
// Returns ok=false when the slug is unknown or bundles.json has not been
// loaded yet (see Load); callers can call Names() to render a useful hint.
func Get(slug string) (Bundle, bool) {
	key := strings.ToLower(strings.TrimSpace(slug))
	m := currentBundles()
	if b, ok := m[key]; ok {
		return b, ok
	}
	if canonical, ok := bundleAliases[key]; ok {
		b, ok := m[canonical]
		return b, ok
	}
	return Bundle{}, false
}

// Names returns every known bundle slug sorted alphabetically. Useful for
// error messages and shell completion. Canonical slugs only — aliases never
// appear here.
func Names() []string {
	m := currentBundles()
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// All returns every Bundle in canonical display order. Bundles not present
// in the loaded set (e.g. a future bundles.json omitting one) are silently
// skipped rather than panicking.
func All() []Bundle {
	m := currentBundles()
	out := make([]Bundle, 0, len(DisplayOrder))
	for _, slug := range DisplayOrder {
		if b, ok := m[slug]; ok {
			out = append(out, b)
		}
	}
	return out
}

// IsInstallable reports whether the bundle can be installed via
// `nself bundle install`. Only the ɳSelf+ meta-bundle (no single SKU, only the
// union of every paid bundle's entitlements) and a bundle with no plugins are
// not installable as a unit. The free Task Bundle is installable since
// P7-PLUG-19 (`nself bundle install task` is its app-ready path); a free bundle
// skips every licence call (see IsFree).
//
// This is the ONLY bundle-membership rule the CLI hand-codes: which display
// slug is a structural category, not plugin data. The plugin SET of every
// installable bundle, including "sentry", is exactly bundles.json's plugins[]
// array: no cli-side subsetting and no special case for nself-stripe or any
// other plugin.
func (b Bundle) IsInstallable() bool {
	if b.Slug == selfPlusSlug {
		return false
	}
	if len(b.Plugins) == 0 {
		return false
	}
	return true
}

// IsFree reports whether the bundle costs nothing. formatPrice in registry.go
// yields "FREE" exactly for a free tier or a zero price.
func (b Bundle) IsFree() bool { return b.Price == "FREE" }

// UnknownBundleError formats a friendly error when a slug does not match a
// known bundle. Includes the sorted list of valid names as a hint.
func UnknownBundleError(slug string) error {
	return fmt.Errorf(
		"unknown bundle %q\n\nAvailable bundles: %s",
		slug, strings.Join(Names(), ", "),
	)
}
