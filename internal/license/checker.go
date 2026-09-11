// Package license — checker.go: bundle-level entitlement check.
//
// BundleEntitled calls ping_api /license/validate?bundle=<name> with the
// operator's license key. This is a DISTINCT call from per-plugin validation:
// a user who has individual plugin entitlements does NOT automatically get
// bundle-level access (bundles are a separate SKU on the ping_api side).
//
// S2.T03 CR-C security requirement: bundle validation MUST use the bundle
// query parameter, NOT individual plugin checks. An operator with a la-carte
// plugin licenses must NOT gain bundle install access for free.
package license

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// bundleValidateResponse is the JSON body from /license/validate?bundle=<name>.
type bundleValidateResponse struct {
	Valid   bool     `json:"valid"`
	Tier    string   `json:"tier"`
	Reason  string   `json:"reason,omitempty"`
	Bundle  string   `json:"bundle,omitempty"`
	Plugins []string `json:"plugins,omitempty"`
}

// BundleEntitled reports whether the operator's license key grants access to
// the named bundle. It calls ping_api /license/validate?bundle=<bundleName>
// with the key.
//
// An AUTHORITATIVE server answer always wins: a 401/403 (key rejected) or a
// 200 with valid:false (server says no) fails CLOSED, full stop — no grace
// applies to those. Grace exists only for the case the server never answered
// at all.
//
// On a NETWORK error (server unreachable), BundleEntitled no longer fails
// closed outright. Per the two-panel licensing review (2026-09-06), punishing
// a paying customer for our own outage is the wrong default, so it consults
// the documented grace-period ladder (grace.go) against the local cache:
//   - cache < GraceSoftThreshold (72h) old: proceed silently.
//   - cache < GraceHardThreshold (7d) old: proceed, warn loudly.
//   - cache >= GraceHardThreshold old, absent, key-mismatched, or the licence
//     is on the revocation list: fail closed — a network outage buys at most
//     7 days, never indefinite access, and revocation is never overridden by
//     grace at any cache age.
//
// FAIL-OPEN exception: when NSELF_LICENSE_FAIL_OPEN=1 is set (CI / air-gap
// environments), the network-unreachable branch instead uses the unbounded
// bundleEntitledFromCache path (tier/sentinel only, no time window) for
// environments that are permanently offline by design. This is now one of
// two grace paths, not the only one — production deployments that leave it
// unset still get the bounded grace ladder above instead of a hard fail.
func BundleEntitled(ctx context.Context, key, bundleName string) (bool, error) {
	if key == "" {
		return false, fmt.Errorf("no license key configured; run 'nself license set <key>' or visit nself.org/pricing")
	}

	pingBase := PingURL()
	url := strings.TrimRight(pingBase, "/") + "/license/validate?bundle=" + bundleName

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(""))
	if err != nil {
		return false, fmt.Errorf("building bundle validation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-NSelf-License-Key", key)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Network unreachable — this is NOT an authoritative rejection, so it
		// gets a grace path rather than an immediate fail-closed.
		//
		// NSELF_LICENSE_FAIL_OPEN=1 keeps its pre-existing unbounded behaviour
		// for deliberately offline CI/air-gap builds. Everyone else falls
		// through to the bounded, revocation-aware grace ladder, which is the
		// default grace path now (see BundleEntitled doc comment).
		if os.Getenv("NSELF_LICENSE_FAIL_OPEN") == "1" {
			return bundleEntitledFromCache(key, bundleName)
		}
		return bundleEntitledFromGrace(key, bundleName)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		// parse and return.
	case http.StatusUnauthorized, http.StatusForbidden:
		return false, fmt.Errorf("license key rejected by server for bundle %q (HTTP %d)", bundleName, resp.StatusCode)
	default:
		return false, fmt.Errorf("unexpected status %d from license server for bundle %q", resp.StatusCode, bundleName)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	if err != nil {
		return false, fmt.Errorf("reading bundle validation response: %w", err)
	}

	var vr bundleValidateResponse
	if err := json.Unmarshal(body, &vr); err != nil {
		return false, fmt.Errorf("decoding bundle validation response: %w", err)
	}

	if !vr.Valid {
		reason := vr.Reason
		if reason == "" {
			reason = "not entitled to this bundle"
		}
		return false, fmt.Errorf("bundle %q: %s (tier=%q)", bundleName, reason, vr.Tier)
	}
	return true, nil
}

// bundleEntitledFromCache is the FAIL-OPEN fallback for air-gap environments.
// It reads the local license cache and checks whether the recorded tier
// covers the requested bundle. This is less authoritative than a live server
// check and must only be used when NSELF_LICENSE_FAIL_OPEN=1 is set.
func bundleEntitledFromCache(key, bundleName string) (bool, error) {
	entry, err := ReadCache()
	if err != nil || entry == nil {
		return false, fmt.Errorf("no license cache available (offline, fail-open attempted for bundle %q)", bundleName)
	}
	if entry.KeyHash != HashKey(key) {
		return false, fmt.Errorf("cached license does not match current key (bundle=%q)", bundleName)
	}

	// Revocation still applies when we cannot reach the server. Without this
	// the fail-open path was the one route by which a revoked licence kept
	// working indefinitely: it validated the key hash and the tier and never
	// consulted the revocation list at all.
	//
	// Only KeyHash is available here. The cache entry carries no JTI, user id
	// or key id, so a revocation published against any of those identifiers
	// cannot be matched on this path; the revocation list must carry a
	// key_hash entry for the refusal to fire. IsRecordRevoked itself remains
	// fail-open for a cold or week-stale cache, which is deliberate.
	if IsRecordRevoked(LicenseRecord{KeyHash: entry.KeyHash}) {
		return false, fmt.Errorf("license has been revoked (bundle=%q); contact support if this is unexpected", bundleName)
	}

	if !cacheCoversBundle(entry, bundleName) {
		return false, fmt.Errorf("bundle %q not found in cached license (tier=%q); connect to validate", bundleName, strings.ToLower(entry.Tier))
	}
	return true, nil
}

// bundleEntitledFromGrace is the DEFAULT network-failure fallback — used
// whenever NSELF_LICENSE_FAIL_OPEN is not set to "1". Unlike
// bundleEntitledFromCache (unbounded in time, reserved for deliberately
// offline CI/air-gap builds), this path is bounded by the documented
// grace-period ladder in grace.go: a cache under GraceSoftThreshold old is
// honoured silently, one under GraceHardThreshold old is honoured with a
// warning, and anything staler — or with WriteAllowed=false, which for a
// bundle *install* (a write against a paid plugin) means the same thing as a
// denial — is refused.
//
// Revocation is checked before the grace window is even consulted: a
// revoked licence gets no grace at any cache age. This reuses
// IsRecordRevoked (internal/license/revocation_refresh_check.go), the same
// revocation-aware check bundleEntitledFromCache already relies on, so both
// grace paths share one revocation rule instead of two.
func bundleEntitledFromGrace(key, bundleName string) (bool, error) {
	entry, err := ReadCache()
	if err != nil || entry == nil {
		return false, fmt.Errorf("license server unreachable and no local license cache available (bundle=%q); connect to the internet or run 'nself license validate'", bundleName)
	}
	if entry.KeyHash != HashKey(key) {
		return false, fmt.Errorf("license server unreachable and cached license does not match current key (bundle=%q)", bundleName)
	}

	// Revocation is never overridden by grace, regardless of cache freshness.
	if IsRecordRevoked(LicenseRecord{KeyHash: entry.KeyHash}) {
		return false, fmt.Errorf("license has been revoked (bundle=%q); contact support if this is unexpected", bundleName)
	}

	grace := DetermineGraceState(entry)
	if !grace.CanProceed || !grace.WriteAllowed {
		return false, fmt.Errorf(
			"license server unreachable (bundle=%q) and the offline grace period has expired: %s",
			bundleName, grace.Message,
		)
	}

	if !cacheCoversBundle(entry, bundleName) {
		return false, fmt.Errorf("license server unreachable; cached license does not cover bundle %q (tier=%q)", bundleName, strings.ToLower(entry.Tier))
	}

	emitGraceWarning(bundleName, grace)
	return true, nil
}

// cacheCoversBundle reports whether a cache entry's tier or explicit
// "bundle:<name>" sentinel covers the named bundle. Shared by the unbounded
// fail-open cache check and the time-bounded grace fallback so the
// entitlement rule has exactly one definition instead of two that can drift.
func cacheCoversBundle(entry *CacheEntry, bundleName string) bool {
	// ɳSelf+ / owner / enterprise cover all bundles. Delegated to IsAllAccessTier
	// so this rule has one definition; the plugin installer needs the same one.
	if IsAllAccessTier(strings.ToLower(entry.Tier)) {
		return true
	}
	// Heuristic: "bundle:<name>" sentinel written by a previous live validation.
	// This is approximate — the authoritative check is the live server call.
	for _, allowed := range entry.PluginsAllowed {
		if strings.EqualFold(allowed, "bundle:"+bundleName) {
			return true
		}
	}
	return false
}

// emitGraceWarning prints a one-line warning to stderr stating that the
// license server was unreachable, that access is continuing on cached
// entitlement, and when the offline grace period runs out. Every grant made
// on the grace path must be announced — never silent.
func emitGraceWarning(bundleName string, grace GraceCheckResult) {
	remaining := GraceHardThreshold - grace.CacheAge
	if remaining < 0 {
		remaining = 0
	}
	fmt.Fprintf(os.Stderr,
		"warning: license server unreachable; continuing bundle %q install on cached entitlement (last validated %s ago). Offline grace period expires in %s.\n",
		bundleName, formatDuration(grace.CacheAge), formatDuration(remaining),
	)
}

// CollectLicenseKey returns the first available license key from env vars or
// key files. This is a convenience helper for callers that need the raw key
// for BundleEntitled without going through the full manager flow.
func CollectLicenseKey() string {
	if k := os.Getenv("NSELF_PLUGIN_LICENSE_KEY_OWNER"); k != "" {
		return k
	}
	if k := os.Getenv("NSELF_PLUGIN_LICENSE_KEY"); k != "" {
		return k
	}
	// Fall back to stored key via manager.
	k, _ := GetKey()
	return k
}
