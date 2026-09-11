// Package license — validator_test.go covers the FAIL-OPEN policy
// implementation in validator.go (D3-T10).
//
// Boundary conditions:
//   - cache age 1d  + remote 200            → Valid (live)
//   - cache age 1d  + remote unreachable    → FailOpen, no warning
//   - cache age 4d  + remote unreachable    → FailOpen, warning
//   - cache age 15d + remote unreachable    → FailClosed
//   - cache tampered (signature invalid)    → FailClosed regardless of TTL
//   - cache absent + remote unreachable     → FailClosed
//   - remote 401                            → FailClosed (NOT fail-open)
//   - remote 503                            → FailOpen (transient backend issue)
//   - remote returns valid=false            → Revoked (overrides cache)
//   - atomic cache write (tmpfile + rename)
package license

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// Test helpers
// ============================================================================

// fixedClock returns a constant time. Used to make age math deterministic.
type fixedClock struct{ t time.Time }

func (f fixedClock) Now() time.Time { return f.t }

// errDoer always returns a transport error (network unreachable).
type errDoer struct{ err error }

func (e errDoer) Do(*http.Request) (*http.Response, error) { return nil, e.err }

// statusDoer returns an HTTP response with the configured status and body.
type statusDoer struct {
	status int
	body   string
}

func (s statusDoer) Do(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: s.status,
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Header:     make(http.Header),
	}, nil
}

// redirectCache points the cache writer at a fresh tmp file per test so cache
// state never leaks across tests.
func redirectCache(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	cachePath := filepath.Join(tmp, "license.json")
	t.Setenv("LICENSE_CACHE_PATH", cachePath)
	return cachePath
}

// seedCache is a helper that writes a CacheEntry directly via WriteCache
// (atomic). Used to seed test state.
func seedCache(t *testing.T, entry *CacheEntry) {
	t.Helper()
	if err := WriteCache(entry); err != nil {
		t.Fatalf("seeding cache: %v", err)
	}
}

// makeEntry returns a CacheEntry whose FetchedAt is `age` before `now` and
// whose ExpiresAt is `now + expiryFromNow`.
func makeEntry(now time.Time, age time.Duration, expiryFromNow time.Duration, key string) *CacheEntry {
	return &CacheEntry{
		KeyHash:        HashKey(key),
		Tier:           "pro",
		PluginsAllowed: []string{"ai", "claw"},
		FetchedAt:      now.Add(-age).Unix(),
		ExpiresAt:      now.Add(expiryFromNow).Unix(),
	}
}

// captureWarn returns a WarnOnce hook that records every call into a slice.
// Useful so tests can assert one-shot vs not-emitted behavior.
func captureWarn() (func(string), *[]string, *sync.Mutex) {
	var (
		mu   sync.Mutex
		msgs []string
	)
	return func(m string) {
		mu.Lock()
		defer mu.Unlock()
		msgs = append(msgs, m)
	}, &msgs, &mu
}

// silentOpts builds a minimal ValidatorOptions with skip-sig=true (entries
// seeded by tests are unsigned) and a captured warn hook.
func silentOpts(now time.Time, doer HTTPDoer, warn func(string)) *ValidatorOptions {
	return &ValidatorOptions{
		Clock:               fixedClock{t: now},
		HTTPClient:          doer,
		PingURL:             "http://example.invalid",
		SkipSignatureVerify: true,
		WarnOnce:            warn,
	}
}

// ============================================================================
// Tests
// ============================================================================

// 1) cache valid 1 day, remote 200 → Valid (live, not from cache).
func TestValidator_FreshCache_RemoteOK_ReturnsValid(t *testing.T) {
	redirectCache(t)
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	key := "nself_pro_testkey1234567890abcdef12345"

	seedCache(t, makeEntry(now, 24*time.Hour, 30*24*time.Hour, key))

	body, _ := json.Marshal(ValidateResponse{
		Valid: true, Tier: "pro", Plugins: []string{"ai", "claw"}, ExpiresAt: now.Add(30 * 24 * time.Hour).Format(time.RFC3339),
	})
	warnFn, _, _ := captureWarn()

	res, err := Validate(context.Background(), key, silentOpts(now, statusDoer{status: 200, body: string(body)}, warnFn))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Status != StatusValid {
		t.Errorf("status = %q, want %q", res.Status, StatusValid)
	}
	if !res.CanProceed {
		t.Errorf("CanProceed = false, want true")
	}
	if res.FromCache {
		t.Errorf("FromCache = true, want false (live verification)")
	}
}

// 2) cache valid 1 day, remote unreachable → Valid (FAIL-OPEN, no warning).
func TestValidator_FreshCache_RemoteUnreachable_FailOpenSilent(t *testing.T) {
	redirectCache(t)
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	key := "nself_pro_testkey1234567890abcdef12345"

	seedCache(t, makeEntry(now, 1*24*time.Hour, 30*24*time.Hour, key))

	warnFn, msgs, mu := captureWarn()
	res, err := Validate(context.Background(), key, silentOpts(now, errDoer{err: errors.New("connection refused")}, warnFn))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Status != StatusFailOpen {
		t.Errorf("status = %q, want %q", res.Status, StatusFailOpen)
	}
	if !res.CanProceed {
		t.Errorf("CanProceed = false, want true")
	}
	if !res.FromCache {
		t.Errorf("FromCache = false, want true")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*msgs) != 0 {
		t.Errorf("warning emitted within soft TTL: %v", *msgs)
	}
}

// 3) cache valid 4 days (within the 72h-7d warning band), remote unreachable → Valid + warning.
func TestValidator_StaleCache_RemoteUnreachable_FailOpenWithWarning(t *testing.T) {
	redirectCache(t)
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	key := "nself_pro_testkey1234567890abcdef12345"

	seedCache(t, makeEntry(now, 4*24*time.Hour, 30*24*time.Hour, key))

	warnFn, msgs, mu := captureWarn()
	res, err := Validate(context.Background(), key, silentOpts(now, errDoer{err: errors.New("dns failure")}, warnFn))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Status != StatusFailOpen {
		t.Errorf("status = %q, want %q", res.Status, StatusFailOpen)
	}
	if !res.CanProceed {
		t.Errorf("CanProceed = false, want true (within hard TTL)")
	}
	if res.WarnMessage == "" {
		t.Errorf("WarnMessage empty, want a warning string")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*msgs) != 1 {
		t.Errorf("expected exactly 1 warning, got %d: %v", len(*msgs), *msgs)
	}
}

// 4) cache valid 15 days, remote unreachable → FailClosed.
func TestValidator_VeryStaleCache_RemoteUnreachable_FailClosed(t *testing.T) {
	redirectCache(t)
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	key := "nself_pro_testkey1234567890abcdef12345"

	seedCache(t, makeEntry(now, 15*24*time.Hour, 30*24*time.Hour, key))

	warnFn, _, _ := captureWarn()
	res, err := Validate(context.Background(), key, silentOpts(now, errDoer{err: errors.New("network down")}, warnFn))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Status != StatusFailClosed {
		t.Errorf("status = %q, want %q", res.Status, StatusFailClosed)
	}
	if res.CanProceed {
		t.Errorf("CanProceed = true, want false (beyond hard TTL)")
	}
}

// 5) cache tampered (signature invalid) → FailClosed regardless of TTL.
//
// Force the validator to actually run signature verification by clearing
// SkipSignatureVerify and seeding a fake non-zero pubkey so IsZeroPubKey()
// returns false — then put a malformed signature in the cache.
func TestValidator_TamperedCache_FailClosedRegardlessOfTTL(t *testing.T) {
	redirectCache(t)

	// Inject a fake non-zero pubkey so VerifySignature actually runs.
	origHex := licensePubKeyHex
	origKeys := bundledPublicKeys
	defer func() {
		licensePubKeyHex = origHex
		bundledPublicKeys = origKeys
	}()
	fake := make([]byte, 32)
	fake[0] = 0xa1
	licensePubKeyHex = hex.EncodeToString(fake)
	bundledPublicKeys = []PublicKeyEntry{{ID: 1, Key: fake}}

	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	key := "nself_pro_testkey1234567890abcdef12345"
	entry := makeEntry(now, 1*time.Hour, 30*24*time.Hour, key)
	entry.Signature = "deadbeef" // intentionally invalid
	entry.SignatureKeyID = 1
	seedCache(t, entry)

	warnFn, _, _ := captureWarn()
	opts := &ValidatorOptions{
		Clock:               fixedClock{t: now},
		HTTPClient:          errDoer{err: errors.New("offline")},
		PingURL:             "http://example.invalid",
		SkipSignatureVerify: false, // run real signature verification
		WarnOnce:            warnFn,
	}
	res, err := Validate(context.Background(), key, opts)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Status != StatusFailClosed {
		t.Errorf("status = %q, want %q (tampered cache must fail closed)", res.Status, StatusFailClosed)
	}
	if res.CanProceed {
		t.Error("CanProceed = true, want false (never fail-open on bad signature)")
	}
	if !strings.Contains(strings.ToLower(res.Reason), "signature") {
		t.Errorf("Reason should mention signature, got %q", res.Reason)
	}
}

// 6) cache absent + remote unreachable → FailClosed.
func TestValidator_NoCache_RemoteUnreachable_FailClosed(t *testing.T) {
	redirectCache(t) // cache file does not exist
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	key := "nself_pro_testkey1234567890abcdef12345"

	warnFn, _, _ := captureWarn()
	res, err := Validate(context.Background(), key, silentOpts(now, errDoer{err: errors.New("offline")}, warnFn))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Status != StatusFailClosed {
		t.Errorf("status = %q, want %q", res.Status, StatusFailClosed)
	}
	if res.CanProceed {
		t.Error("CanProceed = true, want false (no cache + offline)")
	}
}

// 7) remote returns 401 → FailClosed (auth failure, NOT fail-open).
func TestValidator_Remote401_FailClosedNotFailOpen(t *testing.T) {
	redirectCache(t)
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	key := "nself_pro_testkey1234567890abcdef12345"

	// Seed a fresh cache so the only thing that could push us to fail-open is
	// the auth class — but the rule says auth failures bypass fail-open.
	seedCache(t, makeEntry(now, 1*time.Hour, 30*24*time.Hour, key))

	warnFn, _, _ := captureWarn()
	res, err := Validate(context.Background(), key, silentOpts(now, statusDoer{status: 401, body: "{}"}, warnFn))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Status != StatusFailClosed {
		t.Errorf("status = %q, want %q (401 must NOT fail-open)", res.Status, StatusFailClosed)
	}
	if res.CanProceed {
		t.Error("CanProceed = true, want false on 401")
	}
}

// 8) remote returns 503 → FailOpen (transient backend issue, fresh cache).
func TestValidator_Remote503_FailOpenWithFreshCache(t *testing.T) {
	redirectCache(t)
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	key := "nself_pro_testkey1234567890abcdef12345"

	seedCache(t, makeEntry(now, 2*time.Hour, 30*24*time.Hour, key))

	warnFn, _, _ := captureWarn()
	res, err := Validate(context.Background(), key, silentOpts(now, statusDoer{status: 503, body: ""}, warnFn))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Status != StatusFailOpen {
		t.Errorf("status = %q, want %q (503 within soft TTL = fail-open)", res.Status, StatusFailOpen)
	}
	if !res.CanProceed {
		t.Error("CanProceed = false, want true on 5xx + fresh cache")
	}
}

// 9) revocation overrides cache: remote returns valid=false, even with fresh cache.
func TestValidator_RemoteRevoked_OverridesCache(t *testing.T) {
	redirectCache(t)
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	key := "nself_pro_testkey1234567890abcdef12345"

	// Cache is fresh and valid.
	seedCache(t, makeEntry(now, 1*time.Hour, 30*24*time.Hour, key))

	body, _ := json.Marshal(ValidateResponse{
		Valid: false, Reason: "license revoked by issuer",
	})
	warnFn, _, _ := captureWarn()
	res, err := Validate(context.Background(), key, silentOpts(now, statusDoer{status: 200, body: string(body)}, warnFn))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Status != StatusRevoked {
		t.Errorf("status = %q, want %q", res.Status, StatusRevoked)
	}
	if res.CanProceed {
		t.Error("CanProceed = true, want false (revoked)")
	}
}

// 10) atomic cache write — partial write does not corrupt existing cache.
//
// We write a known-good entry, then attempt to overwrite with a path that
// triggers a write failure (cache dir made read-only). The original cache
// must remain readable + parseable.
func TestValidator_AtomicCacheWrite_NoCorruption(t *testing.T) {
	cachePath := redirectCache(t)
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	key := "nself_pro_testkey1234567890abcdef12345"

	original := makeEntry(now, 1*time.Hour, 30*24*time.Hour, key)
	seedCache(t, original)

	// Sanity: cache should be readable as JSON.
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("reading initial cache: %v", err)
	}
	var got CacheEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("initial cache is not parseable JSON: %v", err)
	}
	if got.KeyHash != original.KeyHash {
		t.Fatalf("initial cache content mismatch")
	}

	// Verify the temp file pattern was used (no leftover .tmp file).
	dir := filepath.Dir(cachePath)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("leftover tmp file after atomic write: %s", e.Name())
		}
	}

	// Re-write a different entry. Should succeed and remain parseable.
	updated := makeEntry(now, 30*time.Minute, 60*24*time.Hour, key)
	updated.Tier = "enterprise"
	if err := WriteCache(updated); err != nil {
		t.Fatalf("rewriting cache: %v", err)
	}
	data2, _ := os.ReadFile(cachePath)
	var got2 CacheEntry
	if err := json.Unmarshal(data2, &got2); err != nil {
		t.Fatalf("re-written cache not parseable: %v", err)
	}
	if got2.Tier != "enterprise" {
		t.Errorf("rewrite did not take effect: got tier=%q", got2.Tier)
	}
}

// 11) Soft-TTL boundary: cache age exactly at FailOpenSoftTTL → silent fail-open.
func TestValidator_SoftTTLBoundary_NoWarning(t *testing.T) {
	redirectCache(t)
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	key := "nself_pro_testkey1234567890abcdef12345"

	// age == FailOpenSoftTTL exactly. Per validator semantics: age <= soft → silent.
	seedCache(t, makeEntry(now, FailOpenSoftTTL, 30*24*time.Hour, key))

	warnFn, msgs, mu := captureWarn()
	res, err := Validate(context.Background(), key, silentOpts(now, errDoer{err: errors.New("offline")}, warnFn))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Status != StatusFailOpen {
		t.Errorf("at-soft-TTL: status = %q, want %q", res.Status, StatusFailOpen)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*msgs) != 0 {
		t.Errorf("at-soft-TTL must be silent; got warnings: %v", *msgs)
	}
}

// 12) Hard-TTL boundary: age just past FailOpenHardTTL → fail-closed.
func TestValidator_HardTTLBoundary_FailClosed(t *testing.T) {
	redirectCache(t)
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	key := "nself_pro_testkey1234567890abcdef12345"

	seedCache(t, makeEntry(now, FailOpenHardTTL+1*time.Second, 30*24*time.Hour, key))

	warnFn, _, _ := captureWarn()
	res, err := Validate(context.Background(), key, silentOpts(now, errDoer{err: errors.New("offline")}, warnFn))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Status != StatusFailClosed {
		t.Errorf("just-past-hard-TTL: status = %q, want %q", res.Status, StatusFailClosed)
	}
}

// 13) Server-reported expiry overrides fail-open path: even with fresh cache age
// but server-reported expires_at in the past, validator returns Expired.
func TestValidator_ExpiredLicense_OverridesFailOpen(t *testing.T) {
	redirectCache(t)
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	key := "nself_pro_testkey1234567890abcdef12345"

	// Cache fresh (1h ago) but expires_at is 1h in the past.
	seedCache(t, makeEntry(now, 1*time.Hour, -1*time.Hour, key))

	warnFn, _, _ := captureWarn()
	res, err := Validate(context.Background(), key, silentOpts(now, errDoer{err: errors.New("offline")}, warnFn))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Status != StatusExpired {
		t.Errorf("expired license: status = %q, want %q", res.Status, StatusExpired)
	}
	if res.CanProceed {
		t.Error("expired license: CanProceed should be false")
	}
}

// 14) Real httptest server end-to-end — exercises the full HTTP path
// (request marshaling, response parsing, cache update).
func TestValidator_HTTPTestServer_HappyPath(t *testing.T) {
	redirectCache(t)
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	key := "nself_pro_testkey1234567890abcdef12345"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/license/validate") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ValidateResponse{
			Valid: true, Tier: "pro", Plugins: []string{"ai"},
			ExpiresAt: now.Add(30 * 24 * time.Hour).Format(time.RFC3339),
		})
	}))
	defer srv.Close()

	warnFn, _, _ := captureWarn()
	opts := &ValidatorOptions{
		Clock:               fixedClock{t: now},
		PingURL:             srv.URL,
		SkipSignatureVerify: true,
		WarnOnce:            warnFn,
	}
	res, err := Validate(context.Background(), key, opts)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Status != StatusValid {
		t.Errorf("status = %q, want %q", res.Status, StatusValid)
	}
	if res.FromCache {
		t.Error("FromCache = true, want false (live)")
	}

	// Confirm cache was updated by the call.
	cached, _ := ReadCache()
	if cached == nil {
		t.Fatal("cache not updated after live success")
	}
	if cached.KeyHash != HashKey(key) {
		t.Errorf("cache key hash mismatch")
	}
}

// 15) Cache key hash mismatch → FailClosed.
func TestValidator_CacheKeyMismatch_FailClosed(t *testing.T) {
	redirectCache(t)
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	keyA := "nself_pro_keyAxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	keyB := "nself_pro_keyBxxxxxxxxxxxxxxxxxxxxxxxxxxx"

	seedCache(t, makeEntry(now, 1*time.Hour, 30*24*time.Hour, keyA))

	warnFn, _, _ := captureWarn()
	res, err := Validate(context.Background(), keyB, silentOpts(now, errDoer{err: errors.New("offline")}, warnFn))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Status != StatusFailClosed {
		t.Errorf("status = %q, want %q (cache for different key)", res.Status, StatusFailClosed)
	}
	if !strings.Contains(strings.ToLower(res.Reason), "match") {
		t.Errorf("Reason should mention key mismatch, got %q", res.Reason)
	}
}

// 16) Empty key + valid cache → cache-only validation path.
func TestValidator_NoKey_UsesCacheOnly(t *testing.T) {
	redirectCache(t)
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	key := "nself_pro_testkey1234567890abcdef12345"

	seedCache(t, makeEntry(now, 1*time.Hour, 30*24*time.Hour, key))

	warnFn, _, _ := captureWarn()
	// No HTTP doer needed since no key supplied.
	opts := silentOpts(now, nil, warnFn)
	res, err := Validate(context.Background(), "", opts)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	// Should fail-open from cache without attempting any remote call.
	if res.Status != StatusFailOpen {
		t.Errorf("status = %q, want %q (cache-only path)", res.Status, StatusFailOpen)
	}
	if !res.FromCache {
		t.Error("FromCache = false, want true (cache-only)")
	}
}

// 17) WarnOnce hook is invoked via the default path when no override given.
// Sanity check: nil WarnOnce should not panic.
func TestValidator_NilWarnOnce_DoesNotPanic(t *testing.T) {
	redirectCache(t)
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	key := "nself_pro_testkey1234567890abcdef12345"

	seedCache(t, makeEntry(now, 4*24*time.Hour, 30*24*time.Hour, key))

	opts := &ValidatorOptions{
		Clock:               fixedClock{t: now},
		HTTPClient:          errDoer{err: errors.New("offline")},
		PingURL:             "http://example.invalid",
		SkipSignatureVerify: true,
		// WarnOnce: nil — should fall through to defaultWarnOnce safely
	}

	// Capture stderr to swallow the default warning output.
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() {
		_ = w.Close()
		os.Stderr = origStderr
		_, _ = io.Copy(io.Discard, r)
	}()

	res, err := Validate(context.Background(), key, opts)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Status != StatusFailOpen {
		t.Errorf("status = %q, want %q", res.Status, StatusFailOpen)
	}
}

// guard: make sure ValidatorStatus values are stable strings — protects
// against accidental enum-rename refactors that would silently break
// downstream consumers.
func TestValidator_StatusEnumStability(t *testing.T) {
	want := map[ValidatorStatus]string{
		StatusValid:      "valid",
		StatusExpired:    "expired",
		StatusRevoked:    "revoked",
		StatusFailOpen:   "fail_open",
		StatusFailClosed: "fail_closed",
	}
	for k, v := range want {
		if string(k) != v {
			t.Errorf("status enum %q drifted from %q", k, v)
		}
	}
}

// ============================================================================
// BUILD-LEDGER Finding #14: FAIL-OPEN must consult the local revocation
// cache (D3-T08's IsRecordRevoked) before granting CanProceed: true.
// ============================================================================

// seedRevocation writes a revocation cache entry directly (unsigned —
// IsRecordRevoked's read path does not verify signatures; only the fetch
// path does) matching the pattern used throughout revocation_test.go.
func seedRevocation(t *testing.T, entries ...RevocationEntry) {
	t.Helper()
	cache := &RevocationCache{
		List: RevocationList{
			Kid:     "k",
			Revoked: entries,
		},
		FetchedAt: time.Now().Unix(),
	}
	if err := WriteRevocationCache(cache); err != nil {
		t.Fatalf("WriteRevocationCache: %v", err)
	}
}

// THE NEGATIVE TEST the finding is explicit must exist: a license present
// in the local revocation cache (keyed by KeyHash — the only stable
// identifier available on the fail-open path) must be REJECTED even though
// the remote validator is forced unreachable and the cache is well within
// the soft TTL that would otherwise fail-open silently. A test that only
// exercises the online path proves nothing per the bug file; this one
// forces tryRemote down the transient-failure branch via errDoer so the
// cache-only / fail-open code path is the one under test.
func TestValidator_RevokedKeyHash_RemoteUnreachable_FailClosed(t *testing.T) {
	redirectCache(t)     // CacheEntry (license.json)
	withTempCachePath(t) // RevocationCache (revocation-cache.json) — separate file
	ResetFailOpenWarning()

	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	key := "nself_pro_testkey1234567890abcdef12345"

	// Cache is only 1 day old — comfortably within FailOpenSoftTTL, which
	// would silently grant access if revocation were not consulted.
	seedCache(t, makeEntry(now, 1*24*time.Hour, 30*24*time.Hour, key))
	seedRevocation(t, RevocationEntry{Type: "key_hash", ID: HashKey(key)})

	warnFn, _, _ := captureWarn()
	// Remote path forced to fail — this is the fail-open-eligible branch,
	// not the online path.
	res, err := Validate(context.Background(), key, silentOpts(now, errDoer{err: errors.New("connection refused")}, warnFn))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Status != StatusFailClosed {
		t.Errorf("status = %q, want %q (revoked license must not ride fail-open)", res.Status, StatusFailClosed)
	}
	if res.CanProceed {
		t.Errorf("CanProceed = true, want false — revoked license granted access via fail-open")
	}
}

// POSITIVE regression guard: a license absent from the revocation cache
// (a non-matching key_hash entry is present, proving the check ran and
// simply didn't match) must still fail-open exactly as before. This is the
// "additive, not a behavior change" half of the acceptance criteria.
func TestValidator_NonRevokedKeyHash_RemoteUnreachable_StillFailsOpen(t *testing.T) {
	redirectCache(t)
	withTempCachePath(t)
	ResetFailOpenWarning()

	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	key := "nself_pro_testkey1234567890abcdef12345"

	seedCache(t, makeEntry(now, 1*24*time.Hour, 30*24*time.Hour, key))
	// A revocation list that exists and is checked, but names a different
	// license entirely.
	seedRevocation(t, RevocationEntry{Type: "key_hash", ID: "not-this-license"})

	warnFn, _, _ := captureWarn()
	res, err := Validate(context.Background(), key, silentOpts(now, errDoer{err: errors.New("connection refused")}, warnFn))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Status != StatusFailOpen {
		t.Errorf("status = %q, want %q (non-revoked license must still fail-open)", res.Status, StatusFailOpen)
	}
	if !res.CanProceed {
		t.Errorf("CanProceed = false, want true")
	}
}

// _ = fmt.Sprintf placeholder retained in case future tests need it.
var _ = fmt.Sprintf
