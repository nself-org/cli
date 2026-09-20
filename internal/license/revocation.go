// Package license — revocation.go implements the CLI-side revocation list
// consumer (D3-T08).
//
// The CLI fetches `GET /license/revocation-list` from ping.nself.org hourly
// and stores the signed payload at `~/.config/nself/revocation-cache.json`.
// Every license validation pass consults the local cache to decide whether
// a presented JWT (jti / user_id / kid) or cached license (key_hash) has
// been revoked.
//
// FAIL-OPEN policy (per PPI § Vendor Stack — license validation fail-mode):
//   - Cache up to 7 days stale: still authoritative; refresh attempts run
//     in the background but failures don't block.
//   - Cache > 7 days stale + remote fetch fails: continue treating items
//     as NOT revoked, log a prominent warning. License-server unreachable
//     must never lock paying users out.
//
// Wire format mirrors web/backend/services/ping_api/src/routes/license/
// revocation-list.ts exactly.  Canonical JSON (sorted keys at every level,
// arrays in declared order) is the bytes the server signs and we verify.
package license

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"time"
)

// ─── Constants ────────────────────────────────────────────────────────────────

// RevocationCacheFile is the on-disk location of the cached revocation list.
// Lives under ~/.config/nself so it persists across `nself` upgrades.
const revocationCacheFile = "revocation-cache.json"

// RevocationFailOpenStaleness is the cutoff beyond which a stale cache + a
// failed remote fetch logs a prominent warning.  Within the window, stale
// caches are silently authoritative.
const RevocationFailOpenStaleness = 7 * 24 * time.Hour

// RevocationRefreshInterval is the recommended hourly refresh cadence.
const RevocationRefreshInterval = 1 * time.Hour

// revocationHTTPTimeout caps a single revocation-list fetch.
const revocationHTTPTimeout = 10 * time.Second

// ─── Types ────────────────────────────────────────────────────────────────────

// RevocationEntry is one revoked identifier.  Field order matters for the
// canonical JSON encoder; struct tags pin the wire names.
type RevocationEntry struct {
	Type      string `json:"type"`       // "jti" | "user_id" | "kid" | "key_hash"
	ID        string `json:"id"`         // identifier value
	Reason    string `json:"reason"`     // human-readable
	RevokedAt string `json:"revoked_at"` // ISO8601
}

// RevocationList is the full signed payload from /license/revocation-list.
type RevocationList struct {
	IssuedAt   string            `json:"issued_at"`
	ValidUntil string            `json:"valid_until"`
	Kid        string            `json:"kid"`
	Revoked    []RevocationEntry `json:"revoked"`
	Signature  string            `json:"signature"`
}

// LicenseRecord is the minimal shape needed to evaluate revocation.
// Each field is optional: zero values are treated as "no match attempted".
//
// KeyHash (BUILD-LEDGER Finding #14, closed here): the fail-open cache-only
// path (validator_validate.go) has no JTI/UserID/Kid available — the server's
// ValidateResponse never carries one — but it always has entry.KeyHash
// (HashKey(key), already computed and cached in CacheEntry). KeyHash is
// therefore the join key that lets a server-published revocation list name a
// revoked license without minting a JTI. Do not "simplify" this back to JTI:
// JTI is simply not populated anywhere on the fail-open path.
type LicenseRecord struct {
	JTI     string
	UserID  string
	Kid     string
	KeyHash string
}

// RevocationCache is the on-disk cache wrapper around RevocationList.
// FetchedAt is set by the CLI whenever a successful refresh completes.
// ETag is captured from the most recent 200 response so subsequent refreshes
// can send `If-None-Match` and let the server short-circuit with 304.
// Older caches (pre-D3-T08a) without an ETag field decode with ETag=""
// and simply skip the conditional-GET header — fully backward compatible.
type RevocationCache struct {
	List      RevocationList `json:"list"`
	FetchedAt int64          `json:"fetched_at"`     // unix seconds
	ETag      string         `json:"etag,omitempty"` // server-emitted strong ETag (sha256 hex)
}

// ─── Disk I/O ────────────────────────────────────────────────────────────────

// RevocationCachePath returns the on-disk cache location, honouring
// LICENSE_REVOCATION_CACHE_PATH for tests and unusual deployments.
func RevocationCachePath() (string, error) {
	if p := os.Getenv("LICENSE_REVOCATION_CACHE_PATH"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home directory: %w", err)
	}
	return filepath.Join(home, ".config", "nself", revocationCacheFile), nil
}

// ReadRevocationCache loads the cache from disk.  Returns (nil, nil) when
// the file is absent (cold start).
func ReadRevocationCache() (*RevocationCache, error) {
	path, err := RevocationCachePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// KEEP. A cold start has no revocation cache; that is not a
			// failure and callers already treat (nil, nil) as "no list yet".
			// Note the asymmetry with the licence cache above: a missing
			// revocation list fails OPEN (nothing is known to be revoked), and
			// that is the documented 7-day-window behaviour, not an accident —
			// see IsRecordRevoked in revocation_refresh_check.go. It is also
			// why only ENOENT may take this branch: a revocation list we
			// cannot read must not quietly become a revocation list that is
			// empty.
			return nil, nil
		}
		return nil, fmt.Errorf("reading revocation cache: %w", err)
	}
	var c RevocationCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing revocation cache: %w", err)
	}
	return &c, nil
}

// WriteRevocationCache persists the cache with 0600 permissions.
func WriteRevocationCache(c *RevocationCache) error {
	path, err := RevocationCachePath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := ensureDir(dir); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling revocation cache: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}
