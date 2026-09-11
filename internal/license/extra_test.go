// Package license — extra_test.go covers S193:
// - All-zero pubkey path in VerifySignature
// - NSELF_LICENSE_SKIP_VERIFY gate tests
// - Offline 7d cache TTL (grace state machine)
// - Revocation flow (expired cache = revoke path)
// - FuzzLicenseKeyParser
// - Additional branch coverage for banner, grace helpers, validate helpers
package license

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- S193-T01: All-zero pubkey ---

// TestVerifySignature_ZeroPubKey verifies that a CacheEntry with an empty
// signature fails verification when the dev (zero) pubkey is in use.
func TestVerifySignature_ZeroPubKey(t *testing.T) {
	orig := licensePubKeyHex
	licensePubKeyHex = "" // zero pubkey → IsZeroPubKey() returns true
	defer func() { licensePubKeyHex = orig }()

	entry := &CacheEntry{
		KeyHash:   HashKey("nself_pro_test1234567890abcdef12345"),
		Tier:      "pro",
		FetchedAt: time.Now().Add(-1 * time.Hour).Unix(),
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour).Unix(),
		Signature: "", // no signature
	}
	// With empty sig and zero pubkey, verification must return false.
	if entry.VerifySignature() {
		t.Error("VerifySignature should return false for empty signature with zero pubkey")
	}
}

// TestVerifySignature_InvalidHexSignature verifies that a malformed hex
// signature fails verification gracefully (no panic).
func TestVerifySignature_InvalidHexSignature(t *testing.T) {
	entry := &CacheEntry{
		KeyHash:   HashKey("nself_pro_test1234567890abcdef12345"),
		Tier:      "pro",
		FetchedAt: time.Now().Unix(),
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour).Unix(),
		Signature: "not-valid-hex!!!", // invalid hex
	}
	// Must not panic, must return false.
	if entry.VerifySignature() {
		t.Error("VerifySignature should return false for invalid hex signature")
	}
}

// TestVerifySignature_KeyIDZeroTriesAll verifies the KeyID=0 path: tries all keys.
func TestVerifySignature_KeyIDZero_TriesAll(t *testing.T) {
	entry := &CacheEntry{
		KeyHash:        HashKey("nself_pro_test1234567890abcdef12345"),
		Tier:           "pro",
		FetchedAt:      time.Now().Unix(),
		ExpiresAt:      time.Now().Add(30 * 24 * time.Hour).Unix(),
		Signature:      "", // invalid — will fail
		SignatureKeyID: 0,  // zero means try all keys
	}
	// Must not panic.
	if entry.VerifySignature() {
		t.Error("VerifySignature with keyID=0 and empty signature should return false")
	}
}

// TestVerifySignature_KeyIDMismatch verifies that when the KeyID doesn't
// match the bundled key, VerifySignature falls back to trying all keys.
func TestVerifySignature_KeyIDMismatch(t *testing.T) {
	entry := &CacheEntry{
		KeyHash:        HashKey("nself_pro_test1234567890abcdef12345"),
		Tier:           "pro",
		FetchedAt:      time.Now().Unix(),
		ExpiresAt:      time.Now().Add(30 * 24 * time.Hour).Unix(),
		Signature:      strings.Repeat("ab", 32), // 64 hex chars but invalid sig
		SignatureKeyID: 9999,                     // non-existent key ID
	}
	// Must not panic and must return false (no matching key).
	if entry.VerifySignature() {
		t.Error("VerifySignature with mismatched keyID should return false")
	}
}

// TestGetPublicKeys_Override verifies the LICENSE_PUBLIC_KEY_OVERRIDE env var.
func TestGetPublicKeys_Override(t *testing.T) {
	// Valid 32-byte (64 hex chars) public key.
	validHex := strings.Repeat("ab", 32) // 64 hex chars = 32 bytes
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", validHex)

	keys := GetPublicKeys()
	if len(keys) != 1 {
		t.Fatalf("expected 1 key from override, got %d", len(keys))
	}
	if keys[0].ID != 1 {
		t.Errorf("override key should have ID=1, got %d", keys[0].ID)
	}
}

// TestGetPublicKeys_InvalidOverride verifies that an invalid override hex falls
// back to the bundled keys.
func TestGetPublicKeys_InvalidOverride(t *testing.T) {
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", "not-valid-hex")
	keys := GetPublicKeys()
	// Should fall back to bundled keys (at least 1).
	if len(keys) == 0 {
		t.Error("GetPublicKeys should return bundled keys when override is invalid")
	}
}

// TestGetPublicKeys_WrongLength verifies that a valid hex but wrong-length
// override falls back to bundled keys.
func TestGetPublicKeys_WrongLength(t *testing.T) {
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", "abcd") // only 2 bytes, not 32
	keys := GetPublicKeys()
	if len(keys) == 0 {
		t.Error("GetPublicKeys should return bundled keys when override is wrong length")
	}
}

// --- S193-T02: NSELF_LICENSE_SKIP_VERIFY gate ---

// TestImportCache_InvalidJSON verifies that malformed JSON is rejected.
func TestImportCache_InvalidJSON(t *testing.T) {
	setupCacheDir(t)
	err := ImportCache([]byte("{ not valid json"))
	if err == nil {
		t.Error("ImportCache should return error for invalid JSON")
	}
}

// TestImportCache_MissingRequiredFields verifies that entries without required
// fields (key_hash, tier) are rejected.
func TestImportCache_MissingRequiredFields(t *testing.T) {
	setupCacheDir(t)
	// Entry with empty key_hash and tier.
	entry := CacheEntry{}
	data, _ := json.Marshal(entry)
	err := ImportCache(data)
	if err == nil {
		t.Error("ImportCache should return error for entry with empty key_hash/tier")
	}
	if !strings.Contains(err.Error(), "missing required fields") {
		t.Errorf("expected 'missing required fields' error, got: %v", err)
	}
}

// TestImportCache_SigFailWithoutSkipVerify verifies that unsigned entries are
// rejected when NSELF_LICENSE_SKIP_VERIFY is not set.
func TestImportCache_SigFailWithoutSkipVerify(t *testing.T) {
	setupCacheDir(t)
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY", "")
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY_FORCE", "")

	entry := CacheEntry{
		KeyHash:   HashKey("nself_pro_test1234567890abcdef12345"),
		Tier:      "pro",
		Signature: "", // invalid
	}
	data, _ := json.Marshal(entry)
	err := ImportCache(data)
	if err == nil {
		t.Error("ImportCache should reject unsigned entry when SKIP_VERIFY not set")
	}
	if !strings.Contains(err.Error(), "signature verification failed") {
		t.Errorf("expected 'signature verification failed', got: %v", err)
	}
}

// --- S193-T03: Offline 7d cache TTL ---

// TestOffline7DayCacheTTL_GraceHardState verifies the offline 7-day grace TTL.
// After 7 days offline, the state transitions to GraceHard (writes blocked).
func TestOffline7DayCacheTTL_GraceHardState(t *testing.T) {
	// 7d + 1h past the hard threshold.
	entry := &CacheEntry{
		KeyHash:        HashKey("nself_pro_ttltest1234567890abcdef12"),
		Tier:           "pro",
		PluginsAllowed: []string{"ai", "claw"},
		FetchedAt:      time.Now().Add(-(7*24 + 1) * time.Hour).Unix(),
		ExpiresAt:      time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	result := DetermineGraceState(entry)
	if result.State != GraceHard {
		t.Errorf("7d+1h offline: expected GraceHard, got %s", result.State)
	}
	if result.WriteAllowed {
		t.Error("7d+1h offline: WriteAllowed should be false (GraceHard)")
	}
	if !result.CanProceed {
		t.Error("7d+1h offline: CanProceed should be true (reads still allowed)")
	}
}

// TestOffline7DayCacheTTL_ExactBoundary verifies behavior at the exact 7d mark.
func TestOffline7DayCacheTTL_ExactBoundary(t *testing.T) {
	// Just under 7d — still in soft grace.
	entry := &CacheEntry{
		KeyHash:        HashKey("nself_pro_boundary1234567890abcdef12"),
		Tier:           "pro",
		PluginsAllowed: []string{"ai"},
		FetchedAt:      time.Now().Add(-(7*24 - 1) * time.Hour).Unix(),
		ExpiresAt:      time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	result := DetermineGraceState(entry)
	// 7d-1h: should be soft grace (between 24h and 7d).
	if result.State != GraceSoft {
		t.Errorf("7d-1h offline: expected GraceSoft, got %s", result.State)
	}
	if !result.WriteAllowed {
		t.Errorf("7d-1h offline: WriteAllowed should be true in GraceSoft")
	}
}

// TestOffline7DayCacheTTL_ValidateFull_GraceHardFromCache verifies that
// ValidateFull returns GraceHard (not error) for an 8-day-old cache.
func TestOffline7DayCacheTTL_ValidateFull_GraceHardFromCache(t *testing.T) {
	setupCacheDir(t)
	t.Setenv("LICENSE_PING_URL", "http://127.0.0.1:19999") // unreachable

	const testKey = "nself_pro_gracehard1234567890abcdef12"

	entry := &CacheEntry{
		KeyHash:        HashKey(testKey),
		Tier:           "pro",
		PluginsAllowed: []string{"ai"},
		FetchedAt:      time.Now().Add(-8 * 24 * time.Hour).Unix(), // 8 days ago
		ExpiresAt:      time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	writeCacheRaw(t, entry)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ValidateFull(ctx, testKey)
	if err != nil {
		t.Fatalf("ValidateFull returned unexpected error: %v", err)
	}
	if result.GraceState != GraceHard {
		t.Errorf("expected GraceHard, got %s", result.GraceState)
	}
	if result.WriteAllowed {
		t.Error("WriteAllowed should be false in GraceHard state")
	}
}

// --- S193-T04: Revocation flow ---

// TestRevocation_ExpiredLicenseBlocksAccess verifies that a license expired
// beyond the 30-day post-expiry grace window (PostExpiryGraceWindow,
// P6-E12-W4-S4-T2) causes CanProceed=false in the grace state machine. A
// recently-expired license is NOT blocked — see grace_test.go's
// TestDetermineGraceState_PostExpiryGrace_Day29.
func TestRevocation_ExpiredLicenseBlocksAccess(t *testing.T) {
	// License expired 31 days ago — one day past the 30-day grace window.
	entry := &CacheEntry{
		KeyHash:        HashKey("nself_pro_revoked1234567890abcdef12"),
		Tier:           "pro",
		PluginsAllowed: []string{"ai"},
		FetchedAt:      time.Now().Add(-1 * time.Hour).Unix(),       // fresh cache
		ExpiresAt:      time.Now().Add(-31 * 24 * time.Hour).Unix(), // expired 31d ago
	}
	result := DetermineGraceState(entry)
	if result.State != GraceExpired {
		t.Errorf("expired license: expected GraceExpired, got %s", result.State)
	}
	if result.CanProceed {
		t.Error("expired license: CanProceed should be false")
	}
	if result.WriteAllowed {
		t.Error("expired license: WriteAllowed should be false")
	}
	if result.Message == "" {
		t.Error("expired license: Message should not be empty")
	}
}

// TestRevocation_ValidateFull_ExpiredCacheEntryReturnsInvalid verifies the
// full validation path returns invalid for a license expired beyond the
// 30-day post-expiry grace window (PostExpiryGraceWindow, P6-E12-W4-S4-T2).
func TestRevocation_ValidateFull_ExpiredCacheEntryReturnsInvalid(t *testing.T) {
	setupCacheDir(t)
	t.Setenv("LICENSE_PING_URL", "http://127.0.0.1:19999") // unreachable

	const testKey = "nself_pro_expired1234567890abcdef1234"

	entry := &CacheEntry{
		KeyHash:        HashKey(testKey),
		Tier:           "pro",
		PluginsAllowed: []string{"ai"},
		FetchedAt:      time.Now().Add(-1 * time.Hour).Unix(),
		ExpiresAt:      time.Now().Add(-31 * 24 * time.Hour).Unix(), // expired 31d ago, past grace
	}
	writeCacheRaw(t, entry)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ValidateFull(ctx, testKey)
	if err != nil {
		t.Fatalf("ValidateFull returned unexpected error: %v", err)
	}
	if result.Valid {
		t.Error("ValidateFull should return Valid=false for expired license")
	}
	if result.GraceState != GraceExpired {
		t.Errorf("expected GraceExpired, got %s", result.GraceState)
	}
}

// --- S193-T04 continued: Banner coverage ---

// TestBannerAtWarning_AllStates verifies BannerAtWarning covers all grace states.
func TestBannerAtWarning_AllStates(t *testing.T) {
	tests := []struct {
		state    GraceState
		wantNone bool
	}{
		{GraceValid, true},
		{GraceSoft, false},
		{GraceHard, false},
		{GracePostExpiry, false},
		{GraceExpired, false},
		{GraceRevoked, false},
	}
	for _, tc := range tests {
		result := GraceCheckResult{
			State:     tc.state,
			CacheAge:  2 * 24 * time.Hour,
			ExpiresAt: time.Now().Add(-5 * 24 * time.Hour),
		}
		banner := BannerAtWarning(result)
		if tc.wantNone && banner != "" {
			t.Errorf("state %s: expected empty banner, got %q", tc.state, banner)
		}
		if !tc.wantNone && banner == "" {
			t.Errorf("state %s: expected non-empty banner, got empty", tc.state)
		}
	}
}

// TestBannerAtWarning_GraceSoft_SubDay verifies the "within hours" branch.
func TestBannerAtWarning_GraceSoft_SubDay(t *testing.T) {
	// Cache age so close to GraceHardThreshold that remaining < 24h.
	result := GraceCheckResult{
		State:    GraceSoft,
		CacheAge: GraceHardThreshold - 30*time.Minute, // only 30 min remaining
	}
	banner := BannerAtWarning(result)
	if banner == "" {
		t.Error("GraceSoft sub-day: expected non-empty banner")
	}
	if !strings.Contains(banner, "hours") {
		t.Errorf("GraceSoft sub-day: expected 'hours' in banner, got %q", banner)
	}
}

// TestBannerAtExpiry_NotEmpty verifies the hard-stop banner text is non-empty.
func TestBannerAtExpiry_NotEmpty(t *testing.T) {
	banner := BannerAtExpiry()
	if banner == "" {
		t.Error("BannerAtExpiry should return non-empty string")
	}
	if !strings.Contains(banner, "expired") {
		t.Errorf("BannerAtExpiry should mention 'expired', got %q", banner)
	}
}

// --- S193-T04 continued: Grace helper coverage ---

// TestIsWriteAllowed_AllStates exercises IsWriteAllowed for each grace state.
func TestIsWriteAllowed_AllStates(t *testing.T) {
	tests := []struct {
		state GraceState
		want  bool
	}{
		{GraceValid, true},
		{GraceSoft, true},
		{GraceHard, false},
		{GracePostExpiry, true},
		{GraceExpired, false},
		{GraceRevoked, false},
	}
	for _, tc := range tests {
		r := GraceCheckResult{State: tc.state, WriteAllowed: tc.want}
		if r.IsWriteAllowed() != tc.want {
			t.Errorf("IsWriteAllowed(%s) = %v, want %v", tc.state, r.IsWriteAllowed(), tc.want)
		}
	}
}

// TestNeedsBanner_AllStates exercises NeedsBanner for each grace state.
func TestNeedsBanner_AllStates(t *testing.T) {
	tests := []struct {
		state GraceState
		want  bool
	}{
		{GraceValid, false},
		{GraceSoft, true},
		{GraceHard, true},
		{GracePostExpiry, true},
		{GraceExpired, false},
		{GraceRevoked, false},
	}
	for _, tc := range tests {
		r := GraceCheckResult{State: tc.state}
		if r.NeedsBanner() != tc.want {
			t.Errorf("NeedsBanner(%s) = %v, want %v", tc.state, r.NeedsBanner(), tc.want)
		}
	}
}

// TestFormatDuration_AllBranches exercises all formatDuration branches.
func TestFormatDuration_AllBranches(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Minute, "30 minutes"},
		{1*time.Hour + 30*time.Minute, "1 hours"},
		{47 * time.Hour, "47 hours"},
		{48 * time.Hour, "2 days"},
		{7 * 24 * time.Hour, "7 days"},
	}
	for _, tc := range tests {
		got := formatDuration(tc.d)
		if got != tc.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

// --- S193-T04 continued: validate.go branch coverage ---

// TestPingURL_EnvVarPrecedence verifies the env var priority.
func TestPingURL_EnvVarPrecedence(t *testing.T) {
	t.Setenv("LICENSE_PING_URL", "http://custom.example.com")
	if got := PingURL(); got != "http://custom.example.com" {
		t.Errorf("PingURL() = %q, want http://custom.example.com", got)
	}
}

// TestPingURL_NselfPingAPIURL verifies the fallback env var.
func TestPingURL_NselfPingAPIURL(t *testing.T) {
	t.Setenv("LICENSE_PING_URL", "")
	t.Setenv("NSELF_PING_API_URL", "http://nself-custom.example.com")
	if got := PingURL(); got != "http://nself-custom.example.com" {
		t.Errorf("PingURL() = %q, want http://nself-custom.example.com", got)
	}
}

// TestPingURL_DefaultFallback verifies the production URL default.
func TestPingURL_DefaultFallback(t *testing.T) {
	t.Setenv("LICENSE_PING_URL", "")
	t.Setenv("NSELF_PING_API_URL", "")
	if got := PingURL(); got != DefaultPingURL {
		t.Errorf("PingURL() = %q, want %q", got, DefaultPingURL)
	}
}

// TestRefreshCache_NetworkFailure verifies RefreshCache returns error on network failure.
func TestRefreshCache_NetworkFailure(t *testing.T) {
	setupCacheDir(t)
	t.Setenv("LICENSE_PING_URL", "http://127.0.0.1:19999")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := RefreshCache(ctx, "nself_pro_test1234567890abcdef12345")
	if err == nil {
		t.Error("RefreshCache should return error on network failure")
	}
}

// TestRefreshCache_InvalidResponse verifies RefreshCache handles non-200 responses.
func TestRefreshCache_InvalidResponse(t *testing.T) {
	setupCacheDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := RefreshCache(ctx, "nself_pro_test1234567890abcdef12345")
	if err == nil {
		t.Error("RefreshCache should return error on non-200 response")
	}
}

// TestRefreshCache_InvalidLicense verifies RefreshCache returns not-nil result
// with Valid=false when the server reports invalid.
func TestRefreshCache_InvalidLicense(t *testing.T) {
	setupCacheDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(ValidateResponse{
			Valid:   false,
			Reason:  "revoked",
			Tier:    "",
			Plugins: nil,
		})
	}))
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := RefreshCache(ctx, "nself_pro_test1234567890abcdef12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Valid {
		t.Error("expected Valid=false for revoked key")
	}
}

// TestExportCache_NoCache verifies ExportCache returns error when no cache exists.
func TestExportCache_NoCache(t *testing.T) {
	setupCacheDir(t)
	_, err := ExportCache()
	if err == nil {
		t.Error("ExportCache should return error when no cache file exists")
	}
}

// TestExportCache_WithCache verifies ExportCache returns JSON when cache exists.
func TestExportCache_WithCache(t *testing.T) {
	setupCacheDir(t)

	entry := &CacheEntry{
		KeyHash:        HashKey("nself_pro_export12345678901234567890"),
		Tier:           "pro",
		PluginsAllowed: []string{"ai"},
		FetchedAt:      time.Now().Unix(),
		ExpiresAt:      time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	if err := WriteCache(entry); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}

	data, err := ExportCache()
	if err != nil {
		t.Fatalf("ExportCache: %v", err)
	}
	if len(data) == 0 {
		t.Error("ExportCache returned empty data")
	}
	// Must be valid JSON.
	var check CacheEntry
	if err := json.Unmarshal(data, &check); err != nil {
		t.Errorf("ExportCache returned invalid JSON: %v", err)
	}
}

// TestDeleteCache_NonExistent verifies DeleteCache is a no-op when file is absent.
func TestDeleteCache_NonExistent(t *testing.T) {
	setupCacheDir(t)
	if err := DeleteCache(); err != nil {
		t.Errorf("DeleteCache on absent file should return nil, got: %v", err)
	}
}

// TestDeleteCache_ExistingFile verifies DeleteCache removes the cache file.
func TestDeleteCache_ExistingFile(t *testing.T) {
	setupCacheDir(t)

	entry := &CacheEntry{
		KeyHash:   HashKey("nself_pro_delete12345678901234567890"),
		Tier:      "pro",
		FetchedAt: time.Now().Unix(),
	}
	if err := WriteCache(entry); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}
	if err := DeleteCache(); err != nil {
		t.Fatalf("DeleteCache: %v", err)
	}
	// File should be gone.
	path, _ := CachePath()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("cache file should not exist after DeleteCache")
	}
}

// TestResponseToCache_FullConversion verifies all fields are copied correctly.
func TestResponseToCache_FullConversion(t *testing.T) {
	future := time.Now().Add(30 * 24 * time.Hour)
	resp := &ValidateResponse{
		Valid:     true,
		Tier:      "pro",
		Plugins:   []string{"ai", "claw"},
		ExpiresAt: future.Format(time.RFC3339),
		Signature: "abc123",
		KeyID:     2,
	}
	const key = "nself_pro_resp1234567890abcdef1234567"
	entry := responseToCache(key, resp)
	if entry.KeyHash != HashKey(key) {
		t.Error("responseToCache: KeyHash mismatch")
	}
	if entry.Tier != "pro" {
		t.Errorf("responseToCache: Tier = %q, want pro", entry.Tier)
	}
	if len(entry.PluginsAllowed) != 2 {
		t.Errorf("responseToCache: PluginsAllowed = %v, want 2 entries", entry.PluginsAllowed)
	}
	if entry.ExpiresAt == 0 {
		t.Error("responseToCache: ExpiresAt should be non-zero")
	}
	if entry.Signature != "abc123" {
		t.Errorf("responseToCache: Signature = %q, want abc123", entry.Signature)
	}
	if entry.SignatureKeyID != 2 {
		t.Errorf("responseToCache: SignatureKeyID = %d, want 2", entry.SignatureKeyID)
	}
}

// TestResponseToCache_NoExpiresAt verifies empty ExpiresAt produces zero.
func TestResponseToCache_NoExpiresAt(t *testing.T) {
	resp := &ValidateResponse{
		Valid:     true,
		Tier:      "pro",
		Plugins:   []string{"ai"},
		ExpiresAt: "",
	}
	entry := responseToCache("nself_pro_noexpiry12345678901234567", resp)
	if entry.ExpiresAt != 0 {
		t.Errorf("empty ExpiresAt should produce 0, got %d", entry.ExpiresAt)
	}
}

// TestValidateFull_RemoteSuccess verifies ValidateFull writes cache on success.
func TestValidateFull_RemoteSuccess(t *testing.T) {
	setupCacheDir(t)
	future := time.Now().Add(30 * 24 * time.Hour)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ValidateResponse{
			Valid:     true,
			Tier:      "pro",
			Plugins:   []string{"ai", "claw"},
			ExpiresAt: future.Format(time.RFC3339),
		})
	}))
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const testKey = "nself_pro_success1234567890abcdef1234"
	result, err := ValidateFull(ctx, testKey)
	if err != nil {
		t.Fatalf("ValidateFull: %v", err)
	}
	if !result.Valid {
		t.Errorf("expected Valid=true, got false: %s", result.Message)
	}
	if result.Tier != "pro" {
		t.Errorf("expected Tier=pro, got %q", result.Tier)
	}
	if result.FromCache {
		t.Error("fresh remote result should have FromCache=false")
	}
}

// TestValidateFull_RemoteReturnsInvalid verifies ValidateFull handles server-reported invalid.
func TestValidateFull_RemoteReturnsInvalid(t *testing.T) {
	setupCacheDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ValidateResponse{
			Valid:   false,
			Reason:  "key not found",
			Tier:    "",
			Plugins: nil,
		})
	}))
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := ValidateFull(ctx, "nself_pro_invalid1234567890abcdef1234")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid {
		t.Error("server said invalid — expected Valid=false")
	}
}

// TestHashKey_Deterministic verifies HashKey produces the same output for same input.
func TestHashKey_Deterministic(t *testing.T) {
	key := "nself_pro_hashtest1234567890abcdef12"
	h1 := HashKey(key)
	h2 := HashKey(key)
	if h1 != h2 {
		t.Error("HashKey must be deterministic")
	}
	if len(h1) != 64 { // SHA-256 = 32 bytes = 64 hex chars
		t.Errorf("HashKey should produce 64-char hex, got %d", len(h1))
	}
}

// TestHashKey_DifferentInputs verifies different inputs produce different hashes.
func TestHashKey_DifferentInputs(t *testing.T) {
	h1 := HashKey("nself_pro_keyaaa1234567890abcdef1234")
	h2 := HashKey("nself_pro_keybbb1234567890abcdef1234")
	if h1 == h2 {
		t.Error("different inputs should produce different hashes")
	}
}

// TestCacheAge_Accuracy verifies CacheAge returns approximately the correct duration.
func TestCacheAge_Accuracy(t *testing.T) {
	entry := &CacheEntry{
		FetchedAt: time.Now().Add(-3 * time.Hour).Unix(),
	}
	age := entry.CacheAge()
	if age < 2*time.Hour+55*time.Minute || age > 3*time.Hour+5*time.Minute {
		t.Errorf("CacheAge() = %v, expected ~3h", age)
	}
}

// TestSignablePayload_Format verifies signablePayload produces the canonical format
// including the sorted plugins_allowed field (SIEGE V03-F01 extension).
func TestSignablePayload_Format(t *testing.T) {
	t.Run("no plugins", func(t *testing.T) {
		entry := &CacheEntry{
			KeyHash:   "keyhash",
			Tier:      "pro",
			FetchedAt: 1000,
			ExpiresAt: 2000,
		}
		payload := entry.signablePayload()
		// Empty PluginsAllowed → trailing empty field after final pipe.
		want := "keyhash|pro|1000|2000|"
		if string(payload) != want {
			t.Errorf("signablePayload = %q, want %q", string(payload), want)
		}
	})

	t.Run("plugins sorted in payload", func(t *testing.T) {
		entry := &CacheEntry{
			KeyHash:        "keyhash",
			Tier:           "pro",
			FetchedAt:      1000,
			ExpiresAt:      2000,
			PluginsAllowed: []string{"voice", "ai", "claw"}, // unsorted input
		}
		payload := entry.signablePayload()
		// Must be sorted alphabetically: ai,claw,voice.
		want := "keyhash|pro|1000|2000|ai,claw,voice"
		if string(payload) != want {
			t.Errorf("signablePayload = %q, want %q", string(payload), want)
		}
	})
}

// --- Fuzz target: FuzzLicenseKeyParser ---

// FuzzLicenseKeyParser fuzzes ValidateKeyFormat with arbitrary input.
// Invariants:
//  1. Must not panic.
//  2. Keys with valid prefixes and sufficient length return nil error.
//  3. Empty or too-short keys always return an error.
func FuzzLicenseKeyParser(f *testing.F) {
	// Seed corpus: valid and invalid examples.
	f.Add("")
	f.Add("nself_pro_validkey1234567890abcdefgh")
	f.Add("nself_owner_validkeyabcdef12345678901")
	f.Add("nself_ent_validkeyabcdef1234567890123")
	f.Add("nself_max_validkeyabcdef1234567890123")
	f.Add("no_valid_prefix_1234567890abcdefghijk")
	f.Add("' OR '1'='1'; DROP TABLE users; --")
	f.Add(strings.Repeat("a", 256))
	f.Add("nself_pro_\x00null\x01bytes")
	f.Add("nself_pro_" + strings.Repeat("x", 200))

	f.Fuzz(func(t *testing.T, input string) {
		// Must not panic.
		err := ValidateKeyFormat(input)

		// Empty strings must always return an error.
		if input == "" && err == nil {
			t.Error("empty string should return error from ValidateKeyFormat")
		}

		// Strings shorter than minKeyLength must always return an error.
		if len(input) < minKeyLength && err == nil {
			t.Errorf("short key %q (len=%d < %d) should return error", input, len(input), minKeyLength)
		}
	})
}

// --- helpers ---

// setupCacheDir redirects the license cache to a temp dir.
func setupCacheDir(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	cacheFile := filepath.Join(tmp, "license.json")
	t.Setenv("LICENSE_CACHE_PATH", cacheFile)
}

// writeCacheRaw writes a CacheEntry directly to the redirected cache path,
// bypassing signature verification. Used for testing grace state transitions.
func writeCacheRaw(t *testing.T, entry *CacheEntry) {
	t.Helper()
	path, err := CachePath()
	if err != nil {
		t.Fatalf("CachePath: %v", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
}
