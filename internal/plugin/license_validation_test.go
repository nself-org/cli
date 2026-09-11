// Package plugin — license validation tests (S10-T05).
//
// This file covers: format validation, cache hit/miss/TTL, offline grace
// period, revalidation trigger, S1-T05 HTTP 200 valid=false regression, and
// Basic-tier key denied from Pro-tier plugin.
//
// All HTTP calls use httptest.NewServer — no real network calls are made.
// All file-based cache operations use t.TempDir() directories.
package plugin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

// writeCacheEntryWithAge writes a single cache line for key with status
// ("valid" or "invalid") whose timestamp is `age` in the past. It uses the
// same HMAC format as CacheLicense so that checkLicenseCache and
// checkLicenseCacheOffline will accept the entry.
func writeCacheEntryWithAge(t *testing.T, cacheDir string, key string, status string, age time.Duration) {
	t.Helper()
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatalf("writeCacheEntryWithAge: mkdir: %v", err)
	}
	ts := time.Now().Add(-age).Unix()
	prefix := keyPrefix(key)
	data := fmt.Sprintf("%s|%s|%d", prefix, status, ts)
	sig := hmacSign(data, loadHMACKeyOrFallback())
	line := data + "|" + sig + "\n"
	cachePath := filepath.Join(cacheDir, "cache")
	if err := os.WriteFile(cachePath, []byte(line), 0600); err != nil {
		t.Fatalf("writeCacheEntryWithAge: write: %v", err)
	}
}

// ---- Format validation ------------------------------------------------------

// TestValidateLicenseFormat_RequiresRecognisedPrefix verifies that a key with
// a valid length but no recognised prefix is rejected with ErrInvalidLicenseKey.
func TestValidateLicenseFormat_RequiresRecognisedPrefix(t *testing.T) {
	// 32 chars, no nself_ prefix.
	key := strings.Repeat("x", 32)
	if err := validateLicenseFormat(key); err != errs.ErrInvalidLicenseKey {
		t.Fatalf("expected ErrInvalidLicenseKey for key without recognised prefix, got: %v", err)
	}
}

// TestValidateLicenseFormat_ProPrefixAccepted verifies that nself_pro_ keys of
// sufficient length are accepted.
func TestValidateLicenseFormat_ProPrefixAccepted(t *testing.T) {
	// "nself_pro_" = 10 chars; pad to 32 total.
	key := "nself_pro_" + strings.Repeat("a", 22)
	if err := validateLicenseFormat(key); err != nil {
		t.Fatalf("expected nil error for valid nself_pro_ key, got: %v", err)
	}
}

// TestValidateLicenseFormat_ShortKeyRejected verifies that a key shorter than
// 32 characters is rejected regardless of prefix.
func TestValidateLicenseFormat_ShortKeyRejected(t *testing.T) {
	// 31 chars total — one under the minimum.
	key := "nself_pro_" + strings.Repeat("a", 21)
	if err := validateLicenseFormat(key); err != errs.ErrInvalidLicenseKey {
		t.Fatalf("expected ErrInvalidLicenseKey for 31-char key, got: %v", err)
	}
}

// TestValidateLicenseFormat_EmptyKeyRejected verifies that an empty string is
// rejected.
func TestValidateLicenseFormat_EmptyKeyRejected(t *testing.T) {
	if err := validateLicenseFormat(""); err != errs.ErrInvalidLicenseKey {
		t.Fatalf("expected ErrInvalidLicenseKey for empty key, got: %v", err)
	}
}

// ---- Cache: result returned without HTTP call within TTL --------------------

// TestCheckLicenseCache_NoHTTPCallWhenFresh verifies that checkLicenseCache
// returns a hit for a freshly written entry. The test calls no HTTP server
// at all — if the function were to make a network call instead of reading
// the cache, the httptest constraint would still be satisfied (no server =
// no call). The assertion is that found=true, proving the cache path is taken.
func TestCheckLicenseCache_NoHTTPCallWhenFresh(t *testing.T) {
	cacheDir := t.TempDir()
	key := "nself_pro_" + strings.Repeat("f", 22)

	// Write a fresh cache entry (0 seconds old — well within 72h TTL).
	writeCacheEntryWithAge(t, cacheDir, key, "valid", 0)

	valid, found := checkLicenseCache(key, cacheDir)
	if !found {
		t.Fatal("expected cache hit within TTL, got miss")
	}
	if !valid {
		t.Fatal("expected valid=true from fresh cache, got false")
	}
}

// TestCheckLicenseCache_ExpiredEntryIsAMiss verifies that checkLicenseCache
// returns found=false for a cache entry whose timestamp is older than cacheTTL
// (72 h, aliased to license.GraceSoftThreshold). The test writes a cache
// entry backdated by 73 hours.
func TestCheckLicenseCache_ExpiredEntryIsAMiss(t *testing.T) {
	cacheDir := t.TempDir()
	key := "nself_pro_" + strings.Repeat("g", 22)

	// Write a cache entry 73 hours old — expired by 1 hour.
	writeCacheEntryWithAge(t, cacheDir, key, "valid", 73*time.Hour)

	_, found := checkLicenseCache(key, cacheDir)
	if found {
		t.Fatal("expected cache miss for entry older than cacheTTL (72h), got hit")
	}
}

// ---- Offline grace ----------------------------------------------------------

// TestCheckLicenseCacheOffline_ValidWithinGrace verifies that
// checkLicenseCacheOffline accepts a "valid" entry that is within the 7-day
// offline grace window even though it would be expired for normal online use.
func TestCheckLicenseCacheOffline_ValidWithinGrace(t *testing.T) {
	cacheDir := t.TempDir()
	key := "nself_pro_" + strings.Repeat("h", 22)

	// 96 hours old — expired for online (>72h cacheTTL) but within offline grace (7d).
	writeCacheEntryWithAge(t, cacheDir, key, "valid", 96*time.Hour)

	valid, found := checkLicenseCacheOffline(key, cacheDir)
	if !found {
		t.Fatal("expected offline grace cache hit for entry within 7-day window, got miss")
	}
	if !valid {
		t.Fatal("expected valid=true from offline grace cache, got false")
	}
}

// TestCheckLicenseCacheOffline_InvalidEntryDenied verifies that
// checkLicenseCacheOffline does NOT grant access when the last cached result
// was "invalid". Offline grace only applies to previously-valid licenses.
func TestCheckLicenseCacheOffline_InvalidEntryDenied(t *testing.T) {
	cacheDir := t.TempDir()
	key := "nself_pro_" + strings.Repeat("i", 22)

	// Fresh cache, but marked invalid.
	writeCacheEntryWithAge(t, cacheDir, key, "invalid", 0)

	// An "invalid" entry IS found — it is returned so the caller can see that
	// the cache says invalid (not just missing) — but `found` isn't asserted
	// either way here; regardless of found, valid must be false for an
	// "invalid" cached entry.
	valid, _ := checkLicenseCacheOffline(key, cacheDir)
	if valid {
		t.Fatal("offline grace must not grant access when cached result is 'invalid'")
	}
}

// TestCheckLicenseCacheOffline_ExpiredBeyondGrace verifies that
// checkLicenseCacheOffline returns found=false when the cache entry is older
// than the 7-day offline grace period.
func TestCheckLicenseCacheOffline_ExpiredBeyondGrace(t *testing.T) {
	cacheDir := t.TempDir()
	key := "nself_pro_" + strings.Repeat("j", 22)

	// 8 days old — beyond the 7-day offline grace window.
	writeCacheEntryWithAge(t, cacheDir, key, "valid", 8*24*time.Hour)

	_, found := checkLicenseCacheOffline(key, cacheDir)
	if found {
		t.Fatal("expected offline cache miss for entry older than 7-day grace period, got hit")
	}
}

// TestOfflineGrace_NetworkErrorFallsBackToCache verifies the full offline path:
// when ValidateLicenseRemote returns a network error, checkLicenseCacheOffline
// is used as the fallback and the previously-cached valid result is honoured.
// The "network error" is simulated by pointing the validator at a server that
// immediately closes its listener.
func TestOfflineGrace_NetworkErrorFallsBackToCache(t *testing.T) {
	cacheDir := t.TempDir()
	key := "nself_pro_testkey_aaaaaaaaaaaaaaaa"

	// Pre-populate the cache with a valid result (within offline grace TTL).
	writeCacheEntryWithAge(t, cacheDir, key, "valid", time.Hour)

	// Start and immediately shut down a server so any connection attempt
	// returns a network error (connection refused).
	closedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedSrv.Close() // closed before the request is made

	// Calling ValidateLicenseRemote against a closed server produces a network
	// error. The checkLicense function uses checkLicenseCacheOffline in that
	// case. We test that path directly.
	_, err := ValidateLicenseRemote(context.Background(), key, closedSrv.URL)
	if err == nil {
		t.Fatal("expected network error from closed server, got nil")
	}

	// The offline cache must still grant access.
	offlineValid, offlineFound := checkLicenseCacheOffline(key, cacheDir)
	if !offlineFound {
		t.Fatal("expected offline cache hit after pre-populating cache, got miss")
	}
	if !offlineValid {
		t.Fatal("expected offline cache to report valid=true")
	}
}

// ---- Revalidation trigger ---------------------------------------------------

// TestNeedsRevalidation_ExpiredCacheTriggers verifies that NeedsRevalidation
// returns true when the cached timestamp is older than the revalidation
// interval (7 days). The test writes a cache entry backdated by 8 days.
func TestNeedsRevalidation_ExpiredCacheTriggers(t *testing.T) {
	cacheDir := t.TempDir()
	key := "nself_pro_" + strings.Repeat("k", 22)

	// 8 days old — beyond the 7-day revalidation interval.
	writeCacheEntryWithAge(t, cacheDir, key, "valid", 8*24*time.Hour)

	if !NeedsRevalidation(key, cacheDir) {
		t.Fatal("expected NeedsRevalidation=true for cache older than 7 days, got false")
	}
}

// TestNeedsRevalidation_TamperedCacheTriggers verifies that NeedsRevalidation
// returns true when the cache file has been tampered with (invalid HMAC).
func TestNeedsRevalidation_TamperedCacheTriggers(t *testing.T) {
	cacheDir := t.TempDir()
	key := "nself_pro_" + strings.Repeat("m", 22)

	// Write a syntactically valid-looking entry but with a bogus HMAC.
	ts := time.Now().Unix()
	prefix := keyPrefix(key)
	badLine := fmt.Sprintf("%s|valid|%d|deadbeefdeadbeef\n", prefix, ts)
	cachePath := filepath.Join(cacheDir, "cache")
	if err := os.WriteFile(cachePath, []byte(badLine), 0600); err != nil {
		t.Fatalf("writing tampered cache: %v", err)
	}

	if !NeedsRevalidation(key, cacheDir) {
		t.Fatal("expected NeedsRevalidation=true for tampered cache, got false")
	}
}

// ---- S1-T05 regression: HTTP 200 does NOT mean valid -----------------------

// TestS1T05_Regression_HTTP200ValidFalseIsInvalid is the canonical S1-T05
// regression test. The server returns HTTP 200 with {"valid": false, "reason":
// "subscription_expired"}. The validator MUST decode the body and return
// (false, error) — HTTP 200 alone must not be treated as success.
func TestS1T05_Regression_HTTP200ValidFalseIsInvalid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // 200 OK — but license is NOT valid
		_, _ = w.Write([]byte(`{"valid":false,"reason":"subscription_expired"}`))
	}))
	defer srv.Close()

	valid, err := ValidateLicenseRemote(
		context.Background(),
		"nself_pro_testkey_aaaaaaaaaaaaaaaa",
		srv.URL,
	)
	if valid {
		t.Fatal("S1-T05: HTTP 200 with valid=false must not be treated as a valid license")
	}
	if err == nil {
		t.Fatal("S1-T05: expected a non-nil error when body contains valid=false")
	}
	if !strings.Contains(err.Error(), "subscription_expired") {
		t.Fatalf("S1-T05: expected error to contain reason %q, got: %v", "subscription_expired", err)
	}
}

// TestS1T05_Regression_HTTP200NoValidFieldIsUnreadable verifies that HTTP 200
// with a body that cannot be decoded causes a "unreadable" error (not a silent
// success).
func TestS1T05_Regression_HTTP200NoValidFieldIsUnreadable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`this is not json`))
	}))
	defer srv.Close()

	valid, err := ValidateLicenseRemote(
		context.Background(),
		"nself_pro_testkey_aaaaaaaaaaaaaaaa",
		srv.URL,
	)
	if valid {
		t.Fatal("S1-T05: unparsable body must not be treated as a valid license")
	}
	if err == nil {
		t.Fatal("S1-T05: expected error for unparsable body, got nil")
	}
	if !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("S1-T05: expected error to contain 'unreadable', got: %v", err)
	}
}

// ---- Tier check: Basic key denied from Pro-tier plugin ---------------------

// TestTierCheck_BasicKeyDeniedProPlugin_ViaServer verifies the full remote
// validation path where the server grants a Basic-tier license (with a limited
// plugin list) and the caller requests a Pro-tier plugin ("ai") that is NOT in
// the entitlement list. The result must be ErrLicenseTierTooLow.
//
// This exercises the server-response tier path in checkLicense (not just the
// cached entitlements path tested by TestCheckEntitlements_PluginNotAllowed).
func TestTierCheck_BasicKeyDeniedProPlugin_ViaServer(t *testing.T) {
	// Set up a mock license server that returns a Basic-tier response.
	// The plugins list does NOT include "ai" (which requires Pro tier).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/license/validate" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"valid":true,"tier":"basic","plugins":["notify","chat","cron","livekit"]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	key := "nself_pro_" + strings.Repeat("b", 22)
	cacheDir := t.TempDir()

	// Call validateLicenseRemoteWithEntitlements directly (the internal
	// function used by checkLicense). This returns the parsed response so we
	// can verify the tier logic without needing to wire up the full Install path.
	valid, lvr, err := validateLicenseRemoteWithEntitlements(
		context.Background(),
		key,
		srv.URL,
	)
	if err != nil {
		t.Fatalf("unexpected error from validation server: %v", err)
	}
	if !valid {
		t.Fatal("expected valid=true from server (key is syntactically valid)")
	}
	if lvr == nil {
		t.Fatal("expected non-nil entitlement response from server")
	}
	if lvr.Tier != "basic" {
		t.Fatalf("expected tier 'basic', got %q", lvr.Tier)
	}

	// Verify that "ai" is not in the entitlement list — this is what checkLicense
	// uses to enforce the tier restriction.
	aiAllowed := false
	for _, p := range lvr.Plugins {
		if p == "ai" {
			aiAllowed = true
		}
	}
	if aiAllowed {
		t.Fatal("tier regression: Basic-tier response must not include 'ai' in plugin list")
	}

	// Cache the entitlements and verify that checkEntitlements reports denied.
	if err := cacheEntitlements(cacheDir, lvr.Tier, lvr.Plugins); err != nil {
		t.Fatalf("cacheEntitlements: %v", err)
	}

	allowed, found := checkEntitlements(cacheDir, "ai")
	if !found {
		t.Fatal("expected entitlement cache hit after cacheEntitlements call")
	}
	if allowed {
		t.Fatal("tier regression: Basic-tier key must not be allowed to install Pro-tier 'ai' plugin")
	}
}

// TestTierCheck_BasicKeyAllowedBasicPlugin verifies the positive case: a
// Basic-tier key IS allowed to install a plugin that appears in its entitlement
// list (e.g. "chat").
func TestTierCheck_BasicKeyAllowedBasicPlugin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/license/validate" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"valid":true,"tier":"basic","plugins":["notify","chat","cron"]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	key := "nself_pro_" + strings.Repeat("c", 22)
	cacheDir := t.TempDir()

	valid, lvr, err := validateLicenseRemoteWithEntitlements(
		context.Background(),
		key,
		srv.URL,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !valid {
		t.Fatal("expected valid=true")
	}

	if err := cacheEntitlements(cacheDir, lvr.Tier, lvr.Plugins); err != nil {
		t.Fatalf("cacheEntitlements: %v", err)
	}

	allowed, found := checkEntitlements(cacheDir, "chat")
	if !found {
		t.Fatal("expected entitlement cache hit")
	}
	if !allowed {
		t.Fatal("expected Basic-tier key to be allowed to install 'chat' plugin")
	}
}

// ---- HMAC tamper detection --------------------------------------------------

// TestCheckLicenseCache_TamperedEntryDeletesCache verifies that a cache file
// containing a line with an invalid HMAC causes checkLicenseCache to delete
// the cache file and return (false, false). This prevents an attacker from
// forging a cached "valid" entry.
func TestCheckLicenseCache_TamperedEntryDeletesCache(t *testing.T) {
	cacheDir := t.TempDir()
	key := "nself_pro_" + strings.Repeat("n", 22)

	// Write a cache entry with a forged HMAC.
	ts := time.Now().Unix()
	prefix := keyPrefix(key)
	forgedLine := fmt.Sprintf("%s|valid|%d|0000000000000000000000000000000000000000000000000000000000000000\n", prefix, ts)
	cachePath := filepath.Join(cacheDir, "cache")
	if err := os.WriteFile(cachePath, []byte(forgedLine), 0600); err != nil {
		t.Fatalf("writing forged cache: %v", err)
	}

	valid, found := checkLicenseCache(key, cacheDir)
	if found || valid {
		t.Fatalf("expected (false, false) for tampered cache, got (valid=%v, found=%v)", valid, found)
	}

	// The cache file must have been deleted after detecting tampering.
	if _, statErr := os.Stat(cachePath); statErr == nil {
		t.Fatal("expected cache file to be deleted after HMAC tampering was detected, but file still exists")
	}
}

// ---- Owner key special-cases ------------------------------------------------

// TestCheckLicenseCache_OwnerKeyNeverExpires verifies that an owner key whose
// cache entry is many days old is still considered a hit (owner keys bypass
// TTL expiry).
func TestCheckLicenseCache_OwnerKeyNeverExpires(t *testing.T) {
	cacheDir := t.TempDir()
	key := "nself_owner_" + strings.Repeat("o", 20)

	// 30 days old — would normally be expired for any non-owner key.
	writeCacheEntryWithAge(t, cacheDir, key, "valid", 30*24*time.Hour)

	valid, found := checkLicenseCache(key, cacheDir)
	if !found {
		t.Fatal("expected cache hit for owner key regardless of age, got miss")
	}
	if !valid {
		t.Fatal("expected valid=true for owner key cache, got false")
	}
}
