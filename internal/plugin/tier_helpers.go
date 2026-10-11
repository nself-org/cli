package plugin

// tier_helpers.go — small exported helpers around tier resolution.
//
// Purpose: give callers (cmd/commands plugin info) the licence word for any
//          registry tier and the production entitlement check, without
//          growing tier_resolve.go past the file-size budget.
// Inputs:  a registry tier string; a bundle list.
// Outputs: "free" or "licensed"; an entitlement answer and any lookup error.

import "context"

// LicenseTier is the public licence value (free or licensed) of a registry
// tier: every licensed wire tier (pro, max, ...) is "licensed".
func LicenseTier(t string) string { return LicenseValue(tierClass(t)) }

// DefaultEntitlement is the production EntitlementFunc (the licence check
// ResolvePluginTier uses when given nil), exported so a caller can wrap it
// and see a lookup error that resolution itself fails closed on.
func DefaultEntitlement(ctx context.Context, bundles []string) (bool, error) {
	return defaultEntitlement(ctx, bundles)
}
