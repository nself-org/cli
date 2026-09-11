// Package license — lifecycle_test.go drives the FULL license cache lifecycle
// against a httptest.Server stand-in for ping.nself.org.
//
// Sprint D3 T09 — security-critical E2E (Weight: M).
//
// Coverage (≥10 cases):
//  1. happy path: server up → ValidateFull populates cache → ValidationResult.Valid=true
//  2. fresh cache (1h) + server down → fail-open, GraceValid, FromCache=true
//  3. soft-grace cache (96h) + server down → fail-open with GraceSoft warning
//  4. hard-grace cache (8d, > 7d) + server down → still proceeds but WriteAllowed=false
//  5. expired license + server down → fail-closed, Valid=false
//  6. server reconnect after offline → cache refreshes, GraceValid restored
//  7. server reports invalid (revoked) → CLI blocks (Valid=false)
//  8. cache key mismatch → blocks (cached entry was for different key)
//  9. ping URL via env override (LICENSE_PING_URL) is honoured
//  10. tampered cache field (key_hash mismatch with current key) blocks validation
//  11. RefreshCache fails when server unreachable (no cache update on error)
//  12. ImportCache rejects unsigned cache without skip-verify
package license

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// withCachePath redirects the license cache to a per-test temp path.
// Returns the cache file path so the test can stat / mtime-tweak it.
func withCachePath(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	cachePath := filepath.Join(tmp, "license.json")
	t.Setenv("LICENSE_CACHE_PATH", cachePath)
	return cachePath
}

// validateResponseBody is the JSON shape sent by the canned ping_api.
type validateResponseBody struct {
	Valid     bool     `json:"valid"`
	Reason    string   `json:"reason,omitempty"`
	Tier      string   `json:"tier"`
	Plugins   []string `json:"plugins"`
	ExpiresAt string   `json:"expires_at,omitempty"`
	Signature string   `json:"signature,omitempty"`
	KeyID     int      `json:"key_id,omitempty"`
}

// canned starts an httptest.Server returning the same canned validateResponse
// for every /license/validate POST. Returns the *httptest.Server so the test
// can Close() it to simulate offline.
func canned(t *testing.T, body validateResponseBody, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}))
}

// writeRawCacheEntry writes a cache entry directly bypassing ImportCache so
// we can seed unsigned/expired entries for offline-recovery tests.
func writeRawCacheEntry(t *testing.T, entry *CacheEntry) {
	t.Helper()
	path, err := CachePath()
	if err != nil {
		t.Fatalf("CachePath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

const testKey = "nself_pro_lifecycle_e2e_1234567890abcdef12345"

// ─── Tests ────────────────────────────────────────────────────────────────────

// 1. Happy path: server up → ValidateFull populates cache.
func TestLifecycle_HappyPath_PopulatesCache(t *testing.T) {
	cachePath := withCachePath(t)
	srv := canned(t, validateResponseBody{
		Valid:     true,
		Tier:      "pro",
		Plugins:   []string{"ai", "claw", "mux"},
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339),
	}, http.StatusOK)
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ValidateFull(ctx, testKey)
	if err != nil {
		t.Fatalf("ValidateFull error: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected Valid=true, got false: %s", result.Message)
	}
	if result.Tier != "pro" {
		t.Errorf("expected tier=pro, got %q", result.Tier)
	}
	if result.FromCache {
		t.Error("expected FromCache=false on remote success")
	}
	// Cache file must exist.
	if _, err := os.Stat(cachePath); err != nil {
		t.Errorf("expected cache file at %s, got %v", cachePath, err)
	}
}

// 2. FAIL-OPEN at 1h offline: fresh cache + server down → still valid.
func TestLifecycle_FailOpen_FreshCache_1h(t *testing.T) {
	withCachePath(t)
	srv := canned(t, validateResponseBody{Valid: true, Tier: "pro"}, http.StatusOK)
	t.Setenv("LICENSE_PING_URL", srv.URL)

	now := time.Now()
	writeRawCacheEntry(t, &CacheEntry{
		KeyHash:        HashKey(testKey),
		Tier:           "pro",
		PluginsAllowed: []string{"ai", "claw"},
		FetchedAt:      now.Add(-1 * time.Hour).Unix(),
		ExpiresAt:      now.Add(30 * 24 * time.Hour).Unix(),
	})

	// Stop the server (offline).
	srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := ValidateFull(ctx, testKey)
	if err != nil {
		t.Fatalf("ValidateFull error: %v", err)
	}
	if !result.Valid {
		t.Errorf("FAIL-OPEN broken: 1h cache + offline should still be valid (got %q)", result.Message)
	}
	if !result.FromCache {
		t.Error("expected FromCache=true when offline")
	}
	if result.GraceState != GraceValid {
		t.Errorf("expected GraceState=valid for 1h cache, got %q", result.GraceState)
	}
	if !result.WriteAllowed {
		t.Error("WriteAllowed should be true within soft TTL")
	}
}

// 3. FAIL-OPEN at 96h: cache 72h-7d → GraceSoft (warning) but still valid.
func TestLifecycle_FailOpen_SoftGrace_96h(t *testing.T) {
	withCachePath(t)
	srv := canned(t, validateResponseBody{Valid: true, Tier: "pro"}, http.StatusOK)
	t.Setenv("LICENSE_PING_URL", srv.URL)

	now := time.Now()
	writeRawCacheEntry(t, &CacheEntry{
		KeyHash:        HashKey(testKey),
		Tier:           "pro",
		PluginsAllowed: []string{"ai"},
		FetchedAt:      now.Add(-96 * time.Hour).Unix(),
		ExpiresAt:      now.Add(30 * 24 * time.Hour).Unix(),
	})

	srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := ValidateFull(ctx, testKey)
	if err != nil {
		t.Fatalf("ValidateFull error: %v", err)
	}
	if !result.Valid {
		t.Errorf("expected Valid=true in soft grace, got false: %s", result.Message)
	}
	if result.GraceState != GraceSoft {
		t.Errorf("expected GraceSoft, got %q", result.GraceState)
	}
	if !result.WriteAllowed {
		t.Error("soft grace should still allow writes")
	}
	if result.Message == "" {
		t.Error("expected warning message in soft grace")
	}
}

// 4. Hard grace at 8d (> GraceHardThreshold of 7d): still proceeds but writes blocked.
// Per the GraceCheckResult contract, hard-grace = read-only mode for paid plugins.
func TestLifecycle_HardGrace_8d(t *testing.T) {
	withCachePath(t)
	// Server unreachable.
	t.Setenv("LICENSE_PING_URL", "http://127.0.0.1:19999")

	now := time.Now()
	writeRawCacheEntry(t, &CacheEntry{
		KeyHash:        HashKey(testKey),
		Tier:           "pro",
		PluginsAllowed: []string{"ai"},
		FetchedAt:      now.Add(-8 * 24 * time.Hour).Unix(),
		ExpiresAt:      now.Add(30 * 24 * time.Hour).Unix(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := ValidateFull(ctx, testKey)
	if err != nil {
		t.Fatalf("ValidateFull error: %v", err)
	}
	if !result.Valid {
		t.Errorf("hard grace should still allow read-only proceed, got Valid=false")
	}
	if result.GraceState != GraceHard {
		t.Errorf("expected GraceHard, got %q", result.GraceState)
	}
	if result.WriteAllowed {
		t.Error("hard grace must NOT allow writes")
	}
}

// 5. Expired license (server-reported expiry passed), beyond the 30-day
// post-expiry grace window (PostExpiryGraceWindow, P6-E12-W4-S4-T2) + offline
// → fail-closed. A license expired only recently is NOT fail-closed — see
// TestLifecycle_PostExpiryGrace_StillValid below.
func TestLifecycle_ExpiredLicense_FailClosed(t *testing.T) {
	withCachePath(t)
	t.Setenv("LICENSE_PING_URL", "http://127.0.0.1:19999")

	now := time.Now()
	writeRawCacheEntry(t, &CacheEntry{
		KeyHash:        HashKey(testKey),
		Tier:           "pro",
		PluginsAllowed: []string{"ai"},
		FetchedAt:      now.Add(-1 * time.Hour).Unix(),
		// Expired 31 days ago — one day past the 30-day post-expiry grace window.
		ExpiresAt: now.Add(-31 * 24 * time.Hour).Unix(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := ValidateFull(ctx, testKey)
	if err != nil {
		t.Fatalf("ValidateFull error: %v", err)
	}
	if result.Valid {
		t.Errorf("license expired beyond the grace window must fail-closed, got Valid=true")
	}
	if result.GraceState != GraceExpired {
		t.Errorf("expected GraceExpired, got %q", result.GraceState)
	}
}

// 5b. Expired license within the 30-day post-expiry grace window + offline
// → still proceeds, with a warning (commercial promise, P6-E12-W4-S4-T2).
func TestLifecycle_PostExpiryGrace_StillValid(t *testing.T) {
	withCachePath(t)
	t.Setenv("LICENSE_PING_URL", "http://127.0.0.1:19999")

	now := time.Now()
	writeRawCacheEntry(t, &CacheEntry{
		KeyHash:        HashKey(testKey),
		Tier:           "pro",
		PluginsAllowed: []string{"ai"},
		FetchedAt:      now.Add(-1 * time.Hour).Unix(),
		// Expired 29 days ago — still within the 30-day post-expiry grace window.
		ExpiresAt: now.Add(-29 * 24 * time.Hour).Unix(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := ValidateFull(ctx, testKey)
	if err != nil {
		t.Fatalf("ValidateFull error: %v", err)
	}
	if !result.Valid {
		t.Errorf("license expired 29 days ago must still proceed (within 30-day grace), got Valid=false")
	}
	if result.GraceState != GracePostExpiry {
		t.Errorf("expected GracePostExpiry, got %q", result.GraceState)
	}
	if !result.WriteAllowed {
		t.Error("post-expiry grace should still allow writes")
	}
	if result.Message == "" {
		t.Error("post-expiry grace should carry a warning message")
	}
}

// 6. Server reconnect: after going down + back up, RefreshCache repopulates.
func TestLifecycle_ServerReconnect_RefreshesCache(t *testing.T) {
	withCachePath(t)

	// Phase 1 — server up, prime cache.
	srv := canned(t, validateResponseBody{
		Valid:     true,
		Tier:      "pro",
		Plugins:   []string{"ai"},
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339),
	}, http.StatusOK)
	t.Setenv("LICENSE_PING_URL", srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := ValidateFull(ctx, testKey); err != nil {
		t.Fatalf("phase 1 ValidateFull: %v", err)
	}
	srv.Close()

	// Phase 2 — server back up at fresh URL (simulate reconnect after outage).
	srv2 := canned(t, validateResponseBody{
		Valid:     true,
		Tier:      "max", // tier upgraded server-side
		Plugins:   []string{"ai", "claw"},
		ExpiresAt: time.Now().Add(60 * 24 * time.Hour).UTC().Format(time.RFC3339),
	}, http.StatusOK)
	defer srv2.Close()
	t.Setenv("LICENSE_PING_URL", srv2.URL)

	result, err := RefreshCache(ctx, testKey)
	if err != nil {
		t.Fatalf("RefreshCache after reconnect: %v", err)
	}
	if !result.Valid {
		t.Error("expected Valid=true after reconnect refresh")
	}
	if result.Tier != "max" {
		t.Errorf("expected tier=max after refresh, got %q", result.Tier)
	}
	if result.FromCache {
		t.Error("RefreshCache must hit remote, not cache")
	}
}

// 7. Server reports revoked/invalid → CLI blocks.
func TestLifecycle_ServerReportsRevoked_Blocks(t *testing.T) {
	withCachePath(t)
	srv := canned(t, validateResponseBody{
		Valid:  false,
		Reason: "license revoked",
		Tier:   "pro",
	}, http.StatusOK)
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := ValidateFull(ctx, testKey)
	if err != nil {
		t.Fatalf("ValidateFull: %v", err)
	}
	if result.Valid {
		t.Error("revoked license must NOT be Valid=true")
	}
	if result.Message == "" {
		t.Error("expected revocation reason in Message")
	}
}

// 8. Cache key mismatch: cached entry is for different key → blocks.
func TestLifecycle_CacheKeyMismatch_Blocks(t *testing.T) {
	withCachePath(t)
	t.Setenv("LICENSE_PING_URL", "http://127.0.0.1:19999")

	// Cache holds an entry for a DIFFERENT key.
	now := time.Now()
	writeRawCacheEntry(t, &CacheEntry{
		KeyHash:        HashKey("nself_pro_other_key_0987654321"),
		Tier:           "pro",
		PluginsAllowed: []string{"ai"},
		FetchedAt:      now.Add(-1 * time.Hour).Unix(),
		ExpiresAt:      now.Add(30 * 24 * time.Hour).Unix(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := ValidateFull(ctx, testKey)
	if err != nil {
		t.Fatalf("ValidateFull: %v", err)
	}
	if result.Valid {
		t.Error("cache for a different key must NOT be valid for current key")
	}
}

// 9. LICENSE_PING_URL env override is honoured.
func TestLifecycle_PingURLOverride_Honoured(t *testing.T) {
	withCachePath(t)
	srv := canned(t, validateResponseBody{
		Valid:     true,
		Tier:      "pro",
		Plugins:   []string{"ai"},
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339),
	}, http.StatusOK)
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)
	if got := PingURL(); got != srv.URL {
		t.Errorf("PingURL did not honour env override: got %q, want %q", got, srv.URL)
	}
}

// 10. Tampered cache (mtime within TTL but wrong key_hash) — covered above
// in Cache_Key_Mismatch. Add a complementary test: corrupted JSON yields
// safe behaviour (no panic, returns invalid).
func TestLifecycle_CorruptedCache_NoPanic(t *testing.T) {
	cachePath := withCachePath(t)
	t.Setenv("LICENSE_PING_URL", "http://127.0.0.1:19999")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte("{this is not json"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := ValidateFull(ctx, testKey)
	// Corrupt cache may surface as ReadCache error → ValidateFull returns
	// either an error or an invalid result. Both are acceptable; what we
	// MUST avoid is a panic.
	if err != nil {
		// Acceptable: ReadCache wraps json error.
		return
	}
	if result.Valid {
		t.Error("corrupt cache must not yield Valid=true")
	}
}

// 11. RefreshCache returns error (no cache update) when server unreachable.
func TestLifecycle_RefreshCache_OfflineErrors(t *testing.T) {
	withCachePath(t)
	t.Setenv("LICENSE_PING_URL", "http://127.0.0.1:19999")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := RefreshCache(ctx, testKey); err == nil {
		t.Error("RefreshCache should error when remote unreachable")
	}
}

// 12. Manipulated cache mtime simulating ">7d offline": already covered in
// TestLifecycle_HardGrace_8d above by setting FetchedAt 8 days ago. This
// directly exercises the scenario described in the spec ("manipulate cache
// mtime") since FetchedAt is the canonical staleness field.
//
// 13. ImportCache rejects unsigned cache without --force.
func TestLifecycle_ImportCache_RejectsUnsigned(t *testing.T) {
	withCachePath(t)
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY", "")
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY_FORCE", "")

	entry := CacheEntry{
		KeyHash: HashKey(testKey),
		Tier:    "pro",
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := ImportCache(data); err == nil {
		t.Error("ImportCache must reject unsigned cache without skip-verify")
	}
}
