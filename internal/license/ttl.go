// Package license — ttl.go implements tier-differentiated cache TTL,
// pre-expiry warning thresholds, and signed+compressed cache helpers.
//
// TTL policy (S132-T03):
//
//	Free tier   → instant (no caching, revalidates on every command)
//	Pro tier    → 7 days
//	ɳSelf+/Max  → 30 days
//	Owner       → 30 days
package license

import (
	"strings"
	"time"
)

// Cache TTL durations by tier group.
const (
	// TTLFree is the cache TTL for the free tier (no caching).
	TTLFree = 0

	// TTLPro is the cache TTL for Pro/standard paid tiers.
	TTLPro = 7 * 24 * time.Hour

	// TTLPlus is the cache TTL for ɳSelf+, Max, Enterprise, and Owner tiers.
	TTLPlus = 30 * 24 * time.Hour
)

// Pre-expiry warning thresholds (S132-T01 / T02).
const (
	// PreExpiryWarnStart is when the "N day(s) to expire" warning starts.
	// Must be shorter than the shortest paid TTL (Pro = 7 days) so that a
	// freshly-fetched Pro cache does not immediately trigger the warning.
	// A 2-day window gives enough runway without false positives.
	PreExpiryWarnStart = 2 * 24 * time.Hour

	// PreExpiryWarnEnd is when the warning stops (cache has expired).
	PreExpiryWarnEnd = 0

	// PostExpiryGraceWindow is how long paid plugins keep working after the
	// license itself has expired (server-reported expires_at), before the
	// dormant state begins. This is the commercial promise stated in the
	// Bundle License §4, licensing.mdx, and pricing FAQ: 30 days. Decided
	// P6-E12-W4-S4-T2 2026-09 (the promise wins over the prior 24h value,
	// which was never wired into DetermineGraceState — see grace.go).
	// Unrelated to GraceSoftThreshold/GraceHardThreshold in grace.go (aliased
	// by validator.go's FailOpenSoftTTL/FailOpenHardTTL), which govern the
	// OFFLINE (remote-unreachable) window, not post-expiry grace.
	PostExpiryGraceWindow = 30 * 24 * time.Hour
)

// CacheTTLForTier returns the appropriate cache TTL for a given tier string.
// Tier matching is case-insensitive.
func CacheTTLForTier(tier string) time.Duration {
	t := strings.ToLower(tier)
	switch {
	case t == "" || t == "free":
		return TTLFree
	case strings.Contains(t, "owner"),
		strings.Contains(t, "plus"),
		strings.Contains(t, "+"),
		strings.Contains(t, "max"),
		strings.Contains(t, "enterprise"),
		strings.Contains(t, "business"):
		return TTLPlus
	default:
		// Pro, bundle tiers, and unknowns default to Pro TTL.
		return TTLPro
	}
}

// CacheTTLExpiry returns the time.Time when a cache entry written at
// fetchedAt for the given tier should expire.
func CacheTTLExpiry(tier string, fetchedAt time.Time) time.Time {
	ttl := CacheTTLForTier(tier)
	if ttl == 0 {
		// Free tier: treat as already expired.
		return fetchedAt
	}
	return fetchedAt.Add(ttl)
}

// PreExpiryWarning returns a non-empty message string when the cache age is
// within the PreExpiryWarnStart window but has not yet expired.
// Returns ("", false) when no warning is needed.
//
// S132-T01: pre-expiry warning — "License cache expires in N day(s). Connect
// to revalidate."
// S132-T02: post-expiry grace — "Cache expired. Run nself license revalidate
// after connecting."
func PreExpiryWarning(entry *CacheEntry) (message string, isPostExpiry bool) {
	if entry == nil {
		return "", false
	}

	tier := entry.Tier
	fetchedAt := time.Unix(entry.FetchedAt, 0)
	ttlExpiry := CacheTTLExpiry(tier, fetchedAt)
	now := time.Now()

	// Free tier caches have no TTL to warn about.
	if CacheTTLForTier(tier) == 0 {
		return "", false
	}

	remaining := ttlExpiry.Sub(now)

	switch {
	case remaining <= 0:
		// Post-expiry: cache has expired.
		return "Cache expired. Run nself license revalidate after connecting.", true
	case remaining <= PreExpiryWarnStart:
		// Pre-expiry warning window.
		days := int(remaining.Hours()/24) + 1
		if days == 1 {
			return "License cache expires in 1 day. Connect to revalidate.", false
		}
		return "License cache expires in " + itoa(days) + " days. Connect to revalidate.", false
	default:
		return "", false
	}
}

// itoa converts an int to its decimal string representation without importing
// strconv (avoids a dependency on strconv in this small helper).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
