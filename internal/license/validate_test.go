package license

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// makeFakeEntry returns a minimal CacheEntry JSON with an empty signature.
// VerifySignature() returns false for this entry, making it unsigned.
func makeFakeEntryJSON(t *testing.T) []byte {
	t.Helper()
	entry := CacheEntry{
		KeyHash: HashKey("nself_pro_testkey1234567890abcdef12345"),
		Tier:    "basic",
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshalling fake entry: %v", err)
	}
	return data
}

// redirectCacheDir points the cache writer at a temp dir so tests don't
// corrupt the real cache.
func redirectCacheDir(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("NSELF_CACHE_DIR", filepath.Join(tmp, "cache"))
}

// TestImportCache_SkipVerifyWithoutForceRejected asserts that
// NSELF_LICENSE_SKIP_VERIFY=1 without NSELF_LICENSE_SKIP_VERIFY_FORCE=1 returns
// the expected "requires --force flag" error (S09.T-SEC-01).
func TestImportCache_SkipVerifyWithoutForceRejected(t *testing.T) {
	redirectCacheDir(t)
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY", "1")
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY_FORCE", "")

	data := makeFakeEntryJSON(t)
	err := ImportCache(data)
	if err == nil {
		t.Fatal("expected error when SKIP_VERIFY=1 without FORCE=1, got nil")
	}
	if !strings.Contains(err.Error(), "requires --force flag") {
		t.Errorf("expected 'requires --force flag' in error, got: %q", err.Error())
	}
}

// TestImportCache_SkipVerifyWithForceEmitsWarning asserts that
// NSELF_LICENSE_SKIP_VERIFY=1 WITH NSELF_LICENSE_SKIP_VERIFY_FORCE=1 emits a
// warning to stderr and does NOT return an error for the sig check path
// (S09.T-SEC-01 acceptance: warning emitted to stderr when --force used).
func TestImportCache_SkipVerifyWithForceEmitsWarning(t *testing.T) {
	redirectCacheDir(t)
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY", "1")
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY_FORCE", "1")

	// Capture stderr output.
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	data := makeFakeEntryJSON(t)
	// ImportCache may fail at WriteCache (no real cache dir configured); that's
	// OK — we only care that the skip-verify warning was emitted before any
	// write attempt.
	ImportCache(data) //nolint:errcheck

	_ = w.Close()
	os.Stderr = origStderr
	buf := make([]byte, 512)
	n, _ := r.Read(buf)
	stderrOutput := string(buf[:n])

	if !strings.Contains(stderrOutput, "skip-verify mode") {
		t.Errorf("expected 'skip-verify mode' in stderr warning, got: %q", stderrOutput)
	}
}

// writeCacheEntry writes a CacheEntry to the redirected cache dir for testing.
func writeCacheEntry(t *testing.T, entry *CacheEntry) {
	t.Helper()
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal cache entry: %v", err)
	}
	if err := ImportCache(data); err != nil {
		// ImportCache rejects unsigned entries via sig-verify path.
		// Write directly to cache path instead.
		path, pathErr := CachePath()
		if pathErr != nil {
			t.Fatalf("get cache path: %v", pathErr)
		}
		dir := filepath.Dir(path)
		if mkErr := os.MkdirAll(dir, 0700); mkErr != nil {
			t.Fatalf("mkdir cache dir: %v", mkErr)
		}
		if writeErr := os.WriteFile(path, data, 0600); writeErr != nil {
			t.Fatalf("write cache file: %v", writeErr)
		}
	}
}

// TestDNSFailureFailMode verifies that when the ping.nself.org endpoint is
// unreachable (DNS failure simulation via an unresolvable URL), the license
// validator falls back to the local cache and returns a valid result within
// the configured grace period (7 days / GraceHardThreshold).
//
// Decision: FAIL-OPEN with cached validation (up to 7 days TTL).
// Rationale: fail-closed causes all Pro plugins globally to go dormant during
// any DNS/network outage. Fail-open with a bounded TTL and revocation push
// provides better availability without meaningfully weakening enforcement
// (revocations propagate within 1h via API push when connectivity restores).
//
// S59-T04 acceptance criteria: test simulates DNS failure and verifies cache
// is used within the TTL window.
func TestDNSFailureFailMode(t *testing.T) {
	// Redirect cache so the test does not touch the real ~/.cache/nself dir.
	redirectCacheDir(t)

	// Use a URL that will always refuse connections (localhost unbound port)
	// to simulate DNS/network failure without hitting real Cloudflare DNS.
	t.Setenv("LICENSE_PING_URL", "http://127.0.0.1:19999") // nothing listening

	const testKey = "nself_pro_dnstest1234567890abcdef12345"

	t.Run("no_cache_returns_invalid_on_dns_failure", func(t *testing.T) {
		// With no cache file, DNS failure should return invalid result
		// (not a panic or hang).
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := ValidateFull(ctx, testKey)
		if err != nil {
			t.Fatalf("ValidateFull returned unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("ValidateFull returned nil result")
		}
		// No cache + DNS failure = invalid (no grace possible without a prior validation)
		if result.Valid {
			t.Errorf("expected Valid=false with no cache and DNS failure, got Valid=true")
		}
	})

	t.Run("fresh_cache_valid_on_dns_failure", func(t *testing.T) {
		// Write a fresh cache entry (fetched < 24h ago) for this key.
		now := time.Now()
		entry := &CacheEntry{
			KeyHash:        HashKey(testKey),
			Tier:           "pro",
			PluginsAllowed: []string{"ai", "claw", "mux"},
			FetchedAt:      now.Add(-1 * time.Hour).Unix(), // 1h ago — within GraceSoftThreshold
			ExpiresAt:      now.Add(30 * 24 * time.Hour).Unix(),
		}
		writeCacheEntry(t, entry)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := ValidateFull(ctx, testKey)
		if err != nil {
			t.Fatalf("ValidateFull returned unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("ValidateFull returned nil result")
		}
		// Fresh cache (< 24h) + DNS failure = Valid (GraceValid state)
		if !result.Valid {
			t.Errorf("expected Valid=true with fresh cache and DNS failure, got Valid=false: %s", result.Message)
		}
		if !result.FromCache {
			t.Errorf("expected FromCache=true when remote is unreachable, got FromCache=false")
		}
		if result.GraceState != GraceValid {
			t.Errorf("expected GraceState=%s, got %s", GraceValid, result.GraceState)
		}
	})

	t.Run("stale_cache_grace_soft_on_dns_failure", func(t *testing.T) {
		// Write a stale cache entry (fetched 96h ago — within soft grace window).
		now := time.Now()
		entry := &CacheEntry{
			KeyHash:        HashKey(testKey),
			Tier:           "pro",
			PluginsAllowed: []string{"ai", "claw", "mux"},
			FetchedAt:      now.Add(-96 * time.Hour).Unix(), // 96h ago — GraceSoft range
			ExpiresAt:      now.Add(30 * 24 * time.Hour).Unix(),
		}
		writeCacheEntry(t, entry)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := ValidateFull(ctx, testKey)
		if err != nil {
			t.Fatalf("ValidateFull returned unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("ValidateFull returned nil result")
		}
		// 96h-stale cache + DNS failure = Valid with GraceSoft warning
		if !result.Valid {
			t.Errorf("expected Valid=true in GraceSoft window, got Valid=false: %s", result.Message)
		}
		if result.GraceState != GraceSoft {
			t.Errorf("expected GraceState=%s, got %s", GraceSoft, result.GraceState)
		}
		if !result.WriteAllowed {
			t.Errorf("expected WriteAllowed=true in GraceSoft state, got false")
		}
	})

	t.Run("very_stale_cache_grace_hard_on_dns_failure", func(t *testing.T) {
		// Write a very stale cache entry (8 days ago — beyond GraceHardThreshold).
		now := time.Now()
		entry := &CacheEntry{
			KeyHash:        HashKey(testKey),
			Tier:           "pro",
			PluginsAllowed: []string{"ai", "claw", "mux"},
			FetchedAt:      now.Add(-8 * 24 * time.Hour).Unix(), // 8d ago — beyond GraceHardThreshold
			ExpiresAt:      now.Add(30 * 24 * time.Hour).Unix(),
		}
		writeCacheEntry(t, entry)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := ValidateFull(ctx, testKey)
		if err != nil {
			t.Fatalf("ValidateFull returned unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("ValidateFull returned nil result")
		}
		// 8d-stale cache + DNS failure = GraceHard (read-only, writes blocked)
		if !result.Valid {
			t.Errorf("expected Valid=true in GraceHard window (can still read), got Valid=false: %s", result.Message)
		}
		if result.GraceState != GraceHard {
			t.Errorf("expected GraceState=%s, got %s", GraceHard, result.GraceState)
		}
		if result.WriteAllowed {
			t.Errorf("expected WriteAllowed=false in GraceHard state, got true")
		}
	})

	t.Run("cache_key_mismatch_returns_invalid", func(t *testing.T) {
		// Cache entry exists but for a different key — should return invalid.
		now := time.Now()
		entry := &CacheEntry{
			KeyHash:        HashKey("nself_pro_differentkey1234567890abcdef"),
			Tier:           "pro",
			PluginsAllowed: []string{"ai"},
			FetchedAt:      now.Add(-1 * time.Hour).Unix(),
			ExpiresAt:      now.Add(30 * 24 * time.Hour).Unix(),
		}
		writeCacheEntry(t, entry)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := ValidateFull(ctx, testKey) // testKey != differentkey
		if err != nil {
			t.Fatalf("ValidateFull returned unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("ValidateFull returned nil result")
		}
		if result.Valid {
			t.Errorf("expected Valid=false when cache key doesn't match, got Valid=true")
		}
	})
}

// TestDNSFailureDoesNotHitRealDNS verifies that the test server (simulating
// DNS failure) is actually unreachable — confirming we are not hitting real
// Cloudflare DNS during the test suite.
func TestDNSFailureDoesNotHitRealDNS(t *testing.T) {
	// Start a test HTTP server to confirm what "reachable" looks like.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// The simulation URL used in TestDNSFailureFailMode must NOT be this server.
	simulatedFailURL := "http://127.0.0.1:19999"
	if simulatedFailURL == srv.URL {
		t.Errorf("test collision: simulation URL matches live test server URL %s", srv.URL)
	}
	// Confirm: nothing listens on port 19999 (the connection should be refused).
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, simulatedFailURL+"/health", nil)
	_, err := http.DefaultClient.Do(req)
	if err == nil {
		t.Errorf("expected connection refused on %s (DNS failure simulation), but got a response", simulatedFailURL)
	}
}

// ---------------------------------------------------------------------------
// S10.T03: Ed25519 response signature verification tests
// ---------------------------------------------------------------------------

// makeTestEd25519Keypair generates a fresh Ed25519 keypair for tests and
// injects the public key hex into LICENSE_PUBLIC_KEY_OVERRIDE so GetPublicKeys()
// returns it. Returns the private key (for signing) and a cleanup func.
func makeTestEd25519Keypair(t *testing.T) (privKey ed25519.PrivateKey, pubHex string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate ed25519 keypair: %v", err)
	}
	pubHex = hex.EncodeToString(pub)
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", pubHex)
	return priv, pubHex
}

// makeValidateResponse builds a minimal ValidateResponse JSON body.
func makeValidateResponse(t *testing.T) []byte {
	t.Helper()
	resp := ValidateResponse{
		Valid:   true,
		Tier:    "pro",
		Plugins: []string{"ai", "claw"},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal ValidateResponse: %v", err)
	}
	return data
}

// TestValidateRemote_ValidSignature asserts that a response with a correct
// Ed25519 signature in X-NSelf-License-Sig is accepted and parsed. (S10.T03)
func TestValidateRemote_ValidSignature(t *testing.T) {
	redirectCacheDir(t)
	privKey, _ := makeTestEd25519Keypair(t)

	body := makeValidateResponse(t)
	sig := ed25519.Sign(privKey, body)
	sigHex := hex.EncodeToString(sig)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-NSelf-License-Sig", sigHex)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	t.Setenv("LICENSE_PING_URL", srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := validateRemote(ctx, "nself_pro_testedkey1234567890abcdef12", srv.URL)
	if err != nil {
		t.Fatalf("validateRemote with valid signature returned error: %v", err)
	}
	if result == nil {
		t.Fatal("validateRemote returned nil result")
	}
	if !result.Valid {
		t.Errorf("expected Valid=true, got false; reason=%q", result.Reason)
	}
	if result.Tier != "pro" {
		t.Errorf("expected Tier=pro, got %q", result.Tier)
	}
}

// TestValidateRemote_MissingSignatureRejectsInProdBuild asserts that when the
// X-NSelf-License-Sig header is absent and a real public key is embedded
// (simulated via LICENSE_PUBLIC_KEY_OVERRIDE), validateRemote returns an error
// causing fallback to cache. (S10.T03)
func TestValidateRemote_MissingSignatureRejectsInProdBuild(t *testing.T) {
	redirectCacheDir(t)
	// Inject a real (non-zero) public key so IsZeroPubKey() returns false.
	_, _ = makeTestEd25519Keypair(t)

	body := makeValidateResponse(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Deliberately omit X-NSelf-License-Sig header.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := validateRemote(ctx, "nself_pro_testedkey1234567890abcdef12", srv.URL)
	if err == nil {
		t.Fatal("expected error when X-NSelf-License-Sig is absent, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result on sig failure, got: %+v", result)
	}
	if !strings.Contains(err.Error(), "signature missing") {
		t.Errorf("expected 'signature missing' in error, got: %q", err.Error())
	}
}

// TestValidateRemote_TamperedBodyRejectsInProdBuild asserts that when the
// response body is tampered after signing, validateRemote returns an error.
// This confirms that the signature covers the exact bytes returned. (S10.T03)
func TestValidateRemote_TamperedBodyRejectsInProdBuild(t *testing.T) {
	redirectCacheDir(t)
	privKey, _ := makeTestEd25519Keypair(t)

	originalBody := makeValidateResponse(t)
	sig := ed25519.Sign(privKey, originalBody)
	sigHex := hex.EncodeToString(sig)

	// Tampered: promote tier to "enterprise" without re-signing.
	tamperedResp := ValidateResponse{
		Valid:   true,
		Tier:    "enterprise", // different from what was signed
		Plugins: []string{"ai", "claw"},
	}
	tamperedBody, _ := json.Marshal(tamperedResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-NSelf-License-Sig", sigHex) // sig for original body
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tamperedBody) // different body
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := validateRemote(ctx, "nself_pro_testedkey1234567890abcdef12", srv.URL)
	if err == nil {
		t.Fatal("expected error when response body is tampered, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result on tamper detection, got: %+v", result)
	}
	if !strings.Contains(err.Error(), "signature invalid") {
		t.Errorf("expected 'signature invalid' in error, got: %q", err.Error())
	}
}

// TestValidateRemote_DevBuildSkipsSignatureCheck asserts that when IsZeroPubKey()
// is true (no LICENSE_PUBLIC_KEY_OVERRIDE and empty licensePubKeyHex from ldflags),
// the signature header is not checked — the dev build can talk to any server. (S10.T03)
func TestValidateRemote_DevBuildSkipsSignatureCheck(t *testing.T) {
	redirectCacheDir(t)
	// Do NOT set LICENSE_PUBLIC_KEY_OVERRIDE — IsZeroPubKey() will return true.
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", "")

	body := makeValidateResponse(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// No sig header — would normally fail in prod builds.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := validateRemote(ctx, "nself_pro_testedkey1234567890abcdef12", srv.URL)
	if err != nil {
		t.Fatalf("dev build should skip sig check but got error: %v", err)
	}
	if result == nil {
		t.Fatal("validateRemote returned nil result in dev build")
	}
	if !result.Valid {
		t.Errorf("expected Valid=true in dev build without sig check, got false")
	}
}
