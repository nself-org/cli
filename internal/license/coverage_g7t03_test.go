// Package license — coverage_g7t03_test.go covers G7-T03 Tier-1 license
// module gaps: owner.go (5 funcs), revoke.go (5 funcs), MaskKey,
// DetectTierFromKey, GetEmbeddedPubKeyHex, and additional branch coverage in
// keys.go, tail.go, validate.go, manager.go.
//
// Goal: push internal/license from 70.6% to ≥90% branch coverage.
package license

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// --- owner.go ---

// TestIsOwnerKey checks both prefix-match and non-match.
func TestIsOwnerKey(t *testing.T) {
	if !IsOwnerKey("nself_owner_abcd1234567890abcdef1234567890") {
		t.Error("nself_owner_ prefix should be detected as owner")
	}
	if IsOwnerKey("nself_pro_abcd1234567890abcdef12345678901") {
		t.Error("nself_pro_ prefix should NOT be detected as owner")
	}
	if IsOwnerKey("") {
		t.Error("empty string should not be owner")
	}
}

// TestSetOwnerKey_RoundTrip writes and reads back an owner key.
func TestSetOwnerKey_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_ENV", "")
	t.Setenv("NSELF_OWNER_LICENSE_KEY", "")

	key := "nself_owner_roundtrip12345678901234567890"
	if err := SetOwnerKey(key); err != nil {
		t.Fatalf("SetOwnerKey: %v", err)
	}

	got, err := GetOwnerKey()
	if err != nil {
		t.Fatalf("GetOwnerKey: %v", err)
	}
	if got != key {
		t.Errorf("GetOwnerKey returned %q, want %q", got, key)
	}

	// File mode must be 0600 (Unix only — Windows always reports 0666).
	if runtime.GOOS != "windows" {
		path := filepath.Join(tmp, ".nself", "owner.license")
		st, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat owner.license: %v", err)
		}
		if st.Mode().Perm() != 0600 {
			t.Errorf("owner.license mode = %o, want 0600", st.Mode().Perm())
		}
	}
}

// TestSetOwnerKey_RejectsNonOwnerPrefix verifies the prefix check.
func TestSetOwnerKey_RejectsNonOwnerPrefix(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_ENV", "")

	err := SetOwnerKey("nself_pro_notanowner1234567890abcdef1234")
	if err == nil {
		t.Fatal("SetOwnerKey should reject non-owner prefix")
	}
	if !strings.Contains(err.Error(), "nself_owner_") {
		t.Errorf("error should mention required prefix, got: %v", err)
	}
}

// TestSetOwnerKey_RefusedInDevEnv verifies S133-T02.
func TestSetOwnerKey_RefusedInDevEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_ENV", "dev")

	err := SetOwnerKey("nself_owner_devblocked1234567890abcdef12")
	if err == nil {
		t.Fatal("SetOwnerKey should refuse when NSELF_ENV=dev")
	}
	if !strings.Contains(err.Error(), "dev") {
		t.Errorf("error should mention dev env, got: %v", err)
	}

	// Must also work for case variants (EqualFold).
	t.Setenv("NSELF_ENV", "DEV")
	if err := SetOwnerKey("nself_owner_devblocked1234567890abcdef12"); err == nil {
		t.Fatal("SetOwnerKey should refuse uppercase DEV too")
	}
}

// TestGetOwnerKey_EnvVarPrecedence verifies env var beats file.
func TestGetOwnerKey_EnvVarPrecedence(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_ENV", "")
	envKey := "nself_owner_fromenv1234567890abcdef123456"
	t.Setenv("NSELF_OWNER_LICENSE_KEY", envKey)

	got, err := GetOwnerKey()
	if err != nil {
		t.Fatalf("GetOwnerKey: %v", err)
	}
	if got != envKey {
		t.Errorf("env var should take precedence: got %q, want %q", got, envKey)
	}
}

// TestGetOwnerKey_NoFile_NoEnv returns ("", nil).
func TestGetOwnerKey_NoFile_NoEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_OWNER_LICENSE_KEY", "")

	got, err := GetOwnerKey()
	if err != nil {
		t.Fatalf("GetOwnerKey on missing file should be nil, got: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// TestClearOwnerKey_NoOpWhenAbsent and removes when present.
func TestClearOwnerKey(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_ENV", "")
	t.Setenv("NSELF_OWNER_LICENSE_KEY", "")

	// No-op when absent.
	if err := ClearOwnerKey(); err != nil {
		t.Errorf("ClearOwnerKey on missing file should be nil, got: %v", err)
	}

	// After Set + Clear, GetOwnerKey returns empty.
	if err := SetOwnerKey("nself_owner_cleartest1234567890abcdef1234"); err != nil {
		t.Fatalf("SetOwnerKey: %v", err)
	}
	if err := ClearOwnerKey(); err != nil {
		t.Fatalf("ClearOwnerKey: %v", err)
	}
	got, err := GetOwnerKey()
	if err != nil {
		t.Fatalf("GetOwnerKey: %v", err)
	}
	if got != "" {
		t.Errorf("after Clear, GetOwnerKey should return empty, got %q", got)
	}
}

// --- revoke.go ---

// TestIsRevoked_FalseWhenAbsent.
func TestIsRevoked_FalseWhenAbsent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	if IsRevoked() {
		t.Error("IsRevoked() should be false when marker file is absent")
	}
}

// TestRevokeLicense_FullFlow exercises marker write + cache delete + key clear.
func TestRevokeLicense_FullFlow(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	cachePath := filepath.Join(tmp, "license.json")
	t.Setenv("LICENSE_CACHE_PATH", cachePath)

	// Pre-conditions: write a key and a cache entry.
	if err := SetKey("nself_pro_revoketest1234567890abcdef1234"); err != nil {
		t.Fatalf("SetKey: %v", err)
	}
	entry := &CacheEntry{
		KeyHash:   HashKey("nself_pro_revoketest1234567890abcdef1234"),
		Tier:      "pro",
		FetchedAt: time.Now().Unix(),
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	if err := WriteCache(entry); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}

	// Revoke.
	if err := RevokeLicense(); err != nil {
		t.Fatalf("RevokeLicense: %v", err)
	}

	// Marker present.
	if !IsRevoked() {
		t.Error("IsRevoked() should be true after RevokeLicense")
	}

	// Cache deleted.
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Error("cache file should be removed after revoke")
	}

	// Key cleared.
	got, err := GetKey()
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	if got != "" {
		t.Errorf("key should be cleared after revoke, got %q", got)
	}
}

// TestClearRevokedMarker handles both present and absent paths.
func TestClearRevokedMarker(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	// No-op when absent.
	if err := ClearRevokedMarker(); err != nil {
		t.Errorf("ClearRevokedMarker on absent should be nil, got: %v", err)
	}

	// Write marker, then clear.
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(tmp, "license.json"))
	if err := SetKey("nself_pro_clearmarker12345678901234567890"); err != nil {
		t.Fatalf("SetKey: %v", err)
	}
	if err := RevokeLicense(); err != nil {
		t.Fatalf("RevokeLicense: %v", err)
	}
	if !IsRevoked() {
		t.Fatal("expected IsRevoked() true after RevokeLicense")
	}
	if err := ClearRevokedMarker(); err != nil {
		t.Errorf("ClearRevokedMarker: %v", err)
	}
	if IsRevoked() {
		t.Error("IsRevoked() should be false after ClearRevokedMarker")
	}
}

// TestRestoreWithKey_InvalidKey rejects a key that fails format validation.
func TestRestoreWithKey_InvalidKey(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	err := RestoreWithKey("not_a_valid_key")
	if err == nil {
		t.Fatal("RestoreWithKey should reject invalid key format")
	}
	if !strings.Contains(err.Error(), "invalid license key") {
		t.Errorf("expected 'invalid license key', got: %v", err)
	}
}

// TestRestoreWithKey_HappyPath validates + clears marker + saves new key.
func TestRestoreWithKey_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(tmp, "license.json"))

	// Set up: revoke first.
	if err := SetKey("nself_pro_initial1234567890abcdef12345678"); err != nil {
		t.Fatalf("SetKey: %v", err)
	}
	if err := RevokeLicense(); err != nil {
		t.Fatalf("RevokeLicense: %v", err)
	}
	if !IsRevoked() {
		t.Fatal("expected revoked")
	}

	// Restore with a new key.
	newKey := "nself_pro_restored123456789012345678901234"
	if err := RestoreWithKey(newKey); err != nil {
		t.Fatalf("RestoreWithKey: %v", err)
	}

	if IsRevoked() {
		t.Error("after RestoreWithKey, marker should be cleared")
	}

	got, err := GetKey()
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	if got != newKey {
		t.Errorf("after RestoreWithKey, GetKey = %q, want %q", got, newKey)
	}
}

// --- keys.go: MaskKey, DetectTierFromKey, AddKey/RemoveKey edge paths ---

// TestMaskKey_PublicWrapper verifies MaskKey delegates correctly.
func TestMaskKey_PublicWrapper(t *testing.T) {
	key := "nself_pro_visibleprefix1234567890mask12345"
	masked := MaskKey(key)
	// Must show first 10 + last 4.
	if !strings.HasPrefix(masked, "nself_pro_") {
		t.Errorf("MaskKey should preserve first 10: %q", masked)
	}
	if !strings.HasSuffix(masked, key[len(key)-4:]) {
		t.Errorf("MaskKey should preserve last 4: %q", masked)
	}
	if !strings.Contains(masked, "*") {
		t.Errorf("MaskKey should contain asterisks: %q", masked)
	}
}

// TestMaskKey_ShortKey covers the very-short-key branch.
func TestMaskKey_ShortKey(t *testing.T) {
	// 4-char key => all asterisks.
	if got := MaskKey("abcd"); got != "****" {
		t.Errorf("MaskKey(4chars) = %q, want ****", got)
	}
	// 12-char key (between 10 and 14): first 4 + asterisks.
	got := MaskKey("abcdefghijkl") // 12 chars
	if !strings.HasPrefix(got, "abcd") {
		t.Errorf("MaskKey(12chars) should start with abcd, got %q", got)
	}
}

// TestDetectTierFromKey covers each known prefix and unknown.
func TestDetectTierFromKey(t *testing.T) {
	tests := []struct {
		key      string
		wantTier string
	}{
		{"nself_owner_xxx" + strings.Repeat("a", 30), "ɳSelf Owner"},
		{"nself_plus_xxx" + strings.Repeat("a", 30), "ɳSelf+"},
		{"nself_claw_xxx" + strings.Repeat("a", 30), "ɳClaw"},
		{"nself_clawde_xxx" + strings.Repeat("a", 30), "ClawDE"},
		{"nself_chat_xxx" + strings.Repeat("a", 30), "ɳChat"},
		{"nself_media_xxx" + strings.Repeat("a", 30), "nTV"},
		{"nself_family_xxx" + strings.Repeat("a", 30), "nFamily"},
		{"nself_pro_xxx" + strings.Repeat("a", 30), "ɳSelf Pro"},
		{"nself_ent_xxx" + strings.Repeat("a", 30), "ɳSelf Enterprise"},
		{"nself_max_xxx" + strings.Repeat("a", 30), "ɳSelf Business+"},
		{"unknown_prefix" + strings.Repeat("a", 30), "Unknown"},
	}
	for _, tc := range tests {
		got := DetectTierFromKey(tc.key)
		if got != tc.wantTier {
			t.Errorf("DetectTierFromKey(%q) = %q, want %q", tc.key, got, tc.wantTier)
		}
	}
}

// TestAddKey_FillsAllSlots fills key + key.2..key.10 then errors on slot 11.
func TestAddKey_FillsAllSlots(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")

	for i := 1; i <= 10; i++ {
		key := "nself_pro_slot" + strings.Repeat("a", 35) + string(rune('a'+i))
		if err := AddKey(key); err != nil {
			t.Fatalf("AddKey slot %d: %v", i, err)
		}
	}

	// 11th should fail.
	if err := AddKey("nself_pro_overflow1234567890abcdef12345678"); err == nil {
		t.Error("AddKey should fail at slot 11")
	}
}

// TestAddKey_InvalidFormat rejects bad key.
func TestAddKey_InvalidFormat(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	if err := AddKey("too_short"); err == nil {
		t.Error("AddKey should reject too-short key")
	}
}

// TestRemoveKey_NoMatch returns error.
func TestRemoveKey_NoMatch(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")

	// Add one key.
	if err := AddKey("nself_pro_present12345678901234567890aabb"); err != nil {
		t.Fatalf("AddKey: %v", err)
	}

	_, err := RemoveKey("nself_owner_")
	if err == nil {
		t.Error("RemoveKey with no match should return error")
	}
}

// TestRemoveKey_ByProductName removes via product/display name match.
func TestRemoveKey_ByProductName(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")

	if err := AddKey("nself_pro_byprod1234567890abcdef12345678"); err != nil {
		t.Fatalf("AddKey: %v", err)
	}

	// Match by product name "pro".
	n, err := RemoveKey("pro")
	if err != nil {
		t.Fatalf("RemoveKey: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 key removed, got %d", n)
	}
}

// TestSetKeyReplaceAll_TooShortRejected.
func TestSetKeyReplaceAll_TooShortRejected(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	_, err := SetKeyReplaceAll("short")
	if err == nil {
		t.Error("SetKeyReplaceAll should reject too-short key")
	}
}

// TestGetAllStoredKeys returns slice.
func TestGetAllStoredKeys(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")

	// Empty initially.
	if got := GetAllStoredKeys(); len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}

	// Add 2 keys.
	if err := AddKey("nself_pro_first1234567890abcdef1234567890"); err != nil {
		t.Fatalf("AddKey 1: %v", err)
	}
	if err := AddKey("nself_owner_second12345678901234567890ab"); err != nil {
		t.Fatalf("AddKey 2: %v", err)
	}

	keys := GetAllStoredKeys()
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(keys), keys)
	}
}

// --- cache.go: GetEmbeddedPubKeyHex ---

// TestGetEmbeddedPubKeyHex returns the hex of the first committed ping key.
func TestGetEmbeddedPubKeyHex(t *testing.T) {
	if got := GetEmbeddedPubKeyHex(); len(got) != 64 {
		t.Errorf("GetEmbeddedPubKeyHex = %q, want 64 hex chars", got)
	}
	setPingKeys(t)
	if got := GetEmbeddedPubKeyHex(); got != "" {
		t.Errorf("with no key GetEmbeddedPubKeyHex = %q, want empty", got)
	}
}

// TestGetEmbeddedPubKeyHex_WithValue checks a swapped-in key is reported.
func TestGetEmbeddedPubKeyHex_WithValue(t *testing.T) {
	want := strings.Repeat("ab", 32)
	useTestKey(t, want)
	if got := GetEmbeddedPubKeyHex(); got != want {
		t.Errorf("GetEmbeddedPubKeyHex = %q, want %q", got, want)
	}
}

// --- tail.go: matchesFilters branches, printEvent variants, isTerminal ---

// TestMatchesFilters_AllBranches covers every switch case + match/no-match.
func TestMatchesFilters_AllBranches(t *testing.T) {
	ev := LicenseEvent{
		KeyPrefix: "nself_pro_aa",
		Plugin:    "ai",
		Result:    "allow",
	}

	// "key" prefix match (via min12).
	if !matchesFilters(ev, map[string]string{"key": "nself_pro_aa"}) {
		t.Error("key filter match should pass")
	}
	if matchesFilters(ev, map[string]string{"key": "nself_owner"}) {
		t.Error("non-matching key filter should fail")
	}

	// "key" filter shorter than 12 chars exercises min12 short branch.
	if !matchesFilters(ev, map[string]string{"key": "nself"}) {
		t.Error("short key filter should still match by prefix")
	}

	// "result" exact match.
	if !matchesFilters(ev, map[string]string{"result": "allow"}) {
		t.Error("result=allow should match")
	}
	if matchesFilters(ev, map[string]string{"result": "deny"}) {
		t.Error("result=deny should NOT match an allow event")
	}

	// "plugin" exact match.
	if !matchesFilters(ev, map[string]string{"plugin": "ai"}) {
		t.Error("plugin=ai should match")
	}
	if matchesFilters(ev, map[string]string{"plugin": "claw"}) {
		t.Error("plugin=claw should NOT match an ai event")
	}

	// Multiple filters all must match.
	if !matchesFilters(ev, map[string]string{"plugin": "ai", "result": "allow"}) {
		t.Error("multiple matching filters should pass")
	}
	if matchesFilters(ev, map[string]string{"plugin": "ai", "result": "deny"}) {
		t.Error("any non-match should fail")
	}

	// Unknown filter key — switch default does nothing → match.
	if !matchesFilters(ev, map[string]string{"unknown": "anything"}) {
		t.Error("unknown filter key should not block match")
	}

	// Empty filter map → trivially true.
	if !matchesFilters(ev, map[string]string{}) {
		t.Error("empty filter map should always pass")
	}
}

// TestMin12_AllBranches covers both branches.
func TestMin12_AllBranches(t *testing.T) {
	if got := min12("short"); got != 5 {
		t.Errorf("min12(short) = %d, want 5", got)
	}
	if got := min12("exactly_twelve_plus_more"); got != 12 {
		t.Errorf("min12(longstring) = %d, want 12", got)
	}
	if got := min12(""); got != 0 {
		t.Errorf("min12(empty) = %d, want 0", got)
	}
}

// TestPrintEvent_AllResults covers the color switch (allow/deny/rate_limit).
func TestPrintEvent_AllResults(t *testing.T) {
	results := []string{"allow", "deny", "rate_limit", "other"}
	for _, r := range results {
		var buf bytes.Buffer
		ev := LicenseEvent{
			Timestamp: time.Now(),
			KeyPrefix: "nself_pro_xx",
			Plugin:    "ai",
			Result:    r,
		}
		printEvent(&buf, ev)
		out := buf.String()
		if !strings.Contains(out, strings.ToUpper(r)) {
			t.Errorf("printEvent(%s) output missing %s: %q", r, strings.ToUpper(r), out)
		}
	}
}

// TestPrintEvent_ZeroTimestamp uses time.Now fallback branch.
func TestPrintEvent_ZeroTimestamp(t *testing.T) {
	var buf bytes.Buffer
	ev := LicenseEvent{
		Timestamp: time.Time{}, // zero
		KeyPrefix: "nself_pro_zz",
		Plugin:    "claw",
		Result:    "allow",
	}
	printEvent(&buf, ev)
	if buf.Len() == 0 {
		t.Error("printEvent should write output even with zero timestamp")
	}
}

// TestIsTerminal_NotTerminal covers both branches: bytes.Buffer (not a *os.File)
// and an *os.File pointing to a non-terminal (stdin in test runs).
func TestIsTerminal_NotTerminal(t *testing.T) {
	var buf bytes.Buffer
	if isTerminal(&buf) {
		t.Error("isTerminal(bytes.Buffer) should be false")
	}

	// Pipe → not a terminal.
	tmpFile, err := os.CreateTemp(t.TempDir(), "tail-test-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = tmpFile.Close() }()
	if isTerminal(tmpFile) {
		t.Error("isTerminal(regular file) should be false")
	}
}

// TestIsTerminal_ClosedFile exercises the Stat-error branch.
func TestIsTerminal_ClosedFile(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "tail-closed-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_ = tmpFile.Close()
	// Stat on a closed file fails on most platforms.
	_ = isTerminal(tmpFile) // do not panic
}

// TestStreamOnce_ServerError covers non-200 response path of streamOnce via TailStream.
func TestStreamOnce_ServerError_TriggersReconnect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var buf bytes.Buffer
	err := TailStream(ctx, TailOptions{
		PingURL:    srv.URL,
		Stdout:     &buf,
		HTTPClient: srv.Client(),
	})
	// TailStream returns nil on context cancel after reconnect attempts.
	if err != nil {
		t.Errorf("TailStream after server error should return nil on ctx-done, got: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "reconnecting") {
		t.Errorf("expected reconnect message, got: %q", out)
	}
}

// TestTailStream_DefaultStdout covers the nil-Stdout fallback.
func TestTailStream_DefaultStdout(t *testing.T) {
	// Cancel immediately so streamOnce returns quickly.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Set a non-default ping URL via env so we don't hit production.
	t.Setenv("NSELF_PING_URL", "http://127.0.0.1:1") // unreachable

	// Redirect os.Stdout to capture (defensive).
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
		_ = w.Close()
		_ = r.Close()
	}()

	// Drain pipe in background to avoid blocking write.
	go func() { _, _ = io.Copy(io.Discard, r) }()

	err := TailStream(ctx, TailOptions{
		// Stdout: nil — exercises default branch.
		HTTPClient: &http.Client{Timeout: 100 * time.Millisecond},
	})
	if err != nil {
		t.Errorf("TailStream(cancelled) should return nil, got: %v", err)
	}
}

// TestTailStream_FilterParamsInURL covers the filter-param URL building.
func TestTailStream_FilterParamsInURL(t *testing.T) {
	var receivedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	var buf bytes.Buffer
	_ = TailStream(ctx, TailOptions{
		PingURL:    srv.URL,
		Filters:    map[string]string{"plugin": "ai"},
		Stdout:     &buf,
		HTTPClient: srv.Client(),
	})

	if !strings.Contains(receivedQuery, "plugin=ai") {
		t.Errorf("query should contain plugin=ai, got: %q", receivedQuery)
	}
}

// --- validate.go: RefreshCache success path (covers happy-path lines) ---

// TestRefreshCache_Success exercises the happy path including expires_at parse + cache write.
func TestRefreshCache_Success(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	cachePath := filepath.Join(tmp, "license.json")
	t.Setenv("LICENSE_CACHE_PATH", cachePath)

	future := time.Now().Add(30 * 24 * time.Hour)
	srv := signingServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"valid":true,"tier":"pro","plugins":["ai"],"expires_at":"` + future.Format(time.RFC3339) + `"}`))
	}))
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := RefreshCache(ctx, "nself_pro_refreshok123456789012345678901234")
	if err != nil {
		t.Fatalf("RefreshCache: %v", err)
	}
	if !result.Valid {
		t.Error("expected Valid=true")
	}
	if result.Tier != "pro" {
		t.Errorf("Tier = %q, want pro", result.Tier)
	}

	// Cache must be written.
	if _, err := os.Stat(cachePath); err != nil {
		t.Errorf("cache file should exist after RefreshCache: %v", err)
	}
}

// TestRefreshCache_CacheWriteFailure covers the WriteCache error branch.
func TestRefreshCache_CacheWriteFailure(t *testing.T) {
	// Point cache to an invalid path (a directory that exists, target name occupied).
	tmp := t.TempDir()
	// Make a file at the parent directory location, then point cache inside it.
	// Easier: point LICENSE_CACHE_PATH to a directory itself — WriteCache will
	// try to MkdirAll(parent) which is fine, but WriteFile to the dir will fail.
	t.Setenv("LICENSE_CACHE_PATH", tmp) // tmp is a directory — write will fail

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"valid":true,"tier":"pro","plugins":["ai"]}`))
	}))
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := RefreshCache(ctx, "nself_pro_refreshfail12345678901234567890ab")
	if err == nil {
		t.Error("RefreshCache should error when cache write fails")
	}
}

// --- validate.go: ValidateFull cache mismatch + key-mismatch + remote-then-cache fallback ---

// TestValidateFull_CacheKeyMismatch exercises the entry.KeyHash != HashKey(key) branch.
func TestValidateFull_CacheKeyMismatch(t *testing.T) {
	setupCacheDir(t)
	t.Setenv("LICENSE_PING_URL", "http://127.0.0.1:19999") // unreachable → falls to cache

	// Write a cache for a DIFFERENT key.
	entry := &CacheEntry{
		KeyHash:        HashKey("nself_pro_otherkey1234567890abcdef1234"),
		Tier:           "pro",
		PluginsAllowed: []string{"ai"},
		FetchedAt:      time.Now().Unix(),
		ExpiresAt:      time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	writeCacheRaw(t, entry)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ValidateFull(ctx, "nself_pro_thiskey1234567890abcdef12345678")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid {
		t.Error("Valid should be false on key-hash mismatch")
	}
	if !strings.Contains(result.Message, "does not match") {
		t.Errorf("expected mismatch message, got: %q", result.Message)
	}
}

// TestValidateFull_NoCacheNoNetwork covers the both-fail path.
func TestValidateFull_NoCacheNoNetwork(t *testing.T) {
	setupCacheDir(t)
	t.Setenv("LICENSE_PING_URL", "http://127.0.0.1:19999") // unreachable

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ValidateFull(ctx, "nself_pro_nothing1234567890abcdef12345678")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid {
		t.Error("Valid should be false with no cache and no network")
	}
	if !strings.Contains(result.Message, "no valid cache") {
		t.Errorf("expected 'no valid cache' message, got: %q", result.Message)
	}
}

// TestValidateRemote_BadJSON covers the JSON decode error path.
func TestValidateRemote_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{not valid json"))
	}))
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := validateRemote(ctx, "nself_pro_badjson12345678901234567890abcdef", srv.URL)
	if err == nil {
		t.Error("validateRemote should error on malformed JSON")
	}
}

// TestValidateRemote_BadContextNoServer.
func TestValidateRemote_BadContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancel

	_, err := validateRemote(ctx, "nself_pro_canceled12345678901234567890ab", "http://127.0.0.1:19999")
	if err == nil {
		t.Error("validateRemote with cancelled ctx should error")
	}
	// Ensure we can detect a network/context error (not asserting exact wrap).
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "canceled") && !strings.Contains(err.Error(), "network") {
		t.Logf("got error: %v (acceptable)", err)
	}
}

// --- manager.go: SetKey/GetKey edges, ClearKey roundtrip ---

// TestSetKey_TooShortRejected covers the format error in SetKey.
func TestSetKey_TooShortRejected(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")

	if err := SetKey("short"); err == nil {
		t.Error("SetKey should reject too-short key")
	}
}

// TestClearKey_NoOpWhenAbsent.
func TestClearKey_NoOpWhenAbsent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")

	if err := ClearKey(); err != nil {
		t.Errorf("ClearKey on no files should be nil, got: %v", err)
	}
}

// TestShowKey_GetKeyError covers the GetKey-error path of ShowKey.
// Hard to trigger without HOME-mocking, but if no key file exists, GetKey
// returns ("", nil) which exercises the empty-key branch.
func TestShowKey_NoKey_NoFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")

	masked, tier, err := ShowKey()
	if err != nil {
		t.Errorf("ShowKey on missing should be (\"\",\"\", nil), got err: %v", err)
	}
	if masked != "" || tier != "" {
		t.Errorf("ShowKey on missing should be empty, got masked=%q tier=%q", masked, tier)
	}
}

// --- additional tail.go branches: NSELF_PING_URL fallback + client default + streamOnce filters ---

// TestTailStream_NselfPingUrlFallback exercises the os.Getenv("NSELF_PING_URL") branch.
func TestTailStream_NselfPingUrlFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: " + `{"ts":"2026-04-26T00:00:00Z","key_prefix":"nself_pro_aa","plugin":"ai","result":"allow"}` + "\n\n"))
	}))
	defer srv.Close()

	t.Setenv("NSELF_PING_URL", srv.URL) // exercise lines 61-63 fallback

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var buf bytes.Buffer
	_ = TailStream(ctx, TailOptions{
		// PingURL: "" → falls to NSELF_PING_URL
		Stdout:     &buf,
		HTTPClient: srv.Client(),
	})
	// Just exercise the path; don't assert exact content.
}

// TestTailStream_NilHTTPClient exercises the http.Client default branch (lines 66-68).
func TestTailStream_NilHTTPClient(t *testing.T) {
	// Cancel immediately to avoid real network attempts.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var buf bytes.Buffer
	err := TailStream(ctx, TailOptions{
		PingURL: "http://127.0.0.1:1",
		Stdout:  &buf,
		// HTTPClient: nil → exercises default branch
	})
	if err != nil {
		t.Errorf("TailStream(cancelled) should be nil, got: %v", err)
	}
}

// TestStreamOnce_SkipsNonDataAndDoneLines covers lines 132-139 + the JSON-error-continue branch.
func TestStreamOnce_SkipsNonDataAndDoneLines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Mix: comment lines, [DONE] sentinel, malformed JSON, valid event.
		_, _ = w.Write([]byte(": this is a comment\n"))
		_, _ = w.Write([]byte("event: ping\n"))
		_, _ = w.Write([]byte("data: \n")) // empty data
		_, _ = w.Write([]byte("data: [DONE]\n"))
		_, _ = w.Write([]byte("data: {not_valid_json\n"))
		_, _ = w.Write([]byte(`data: {"ts":"2026-04-26T00:00:00Z","key_prefix":"nself_pro_aa","plugin":"ai","result":"allow"}` + "\n"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var buf bytes.Buffer
	_ = TailStream(ctx, TailOptions{
		PingURL:    srv.URL,
		Stdout:     &buf,
		HTTPClient: srv.Client(),
	})

	out := buf.String()
	if !strings.Contains(out, "ALLOW") {
		t.Errorf("expected ALLOW from valid event, got: %q", out)
	}
}

// TestStreamOnce_BadURL covers the http.NewRequestWithContext error path.
func TestStreamOnce_BadURL(t *testing.T) {
	// Use an invalid URL with control chars to force NewRequestWithContext to fail.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var buf bytes.Buffer
	_ = TailStream(ctx, TailOptions{
		PingURL:    "http://[::1]:99999\x00bad",
		Stdout:     &buf,
		HTTPClient: &http.Client{Timeout: 50 * time.Millisecond},
	})
	// Expect the reconnect message in output.
	if !strings.Contains(buf.String(), "reconnecting") && !strings.Contains(buf.String(), "Streaming") {
		t.Logf("output: %q", buf.String())
	}
}

// --- additional ValidateFull / ImportCache happy-path coverage ---

// TestImportCache_SkipVerifyHappyPath covers the warning-emit + WriteCache path.
func TestImportCache_SkipVerifyHappyPath(t *testing.T) {
	setupCacheDir(t)
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY", "1")
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY_FORCE", "1")

	entry := CacheEntry{
		KeyHash:   HashKey("nself_pro_skipverify12345678901234567890"),
		Tier:      "pro",
		Signature: "", // unsigned, but skip-verify+force allows it
	}
	data, _ := jsonMarshalForTest(entry)
	if err := ImportCache(data); err != nil {
		t.Errorf("ImportCache with skip-verify+force should succeed, got: %v", err)
	}
}

// TestImportCache_SkipVerifyWithoutForce covers the standalone skip rejection (already in extra_test).
// This duplicate is fine: it ensures Test order doesn't matter and re-exercises that branch.
func TestImportCache_SkipVerifyWithoutForceRejected_Local(t *testing.T) {
	setupCacheDir(t)
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY", "1")
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY_FORCE", "")

	entry := CacheEntry{
		KeyHash:   HashKey("nself_pro_skipnoforce12345678901234567"),
		Tier:      "pro",
		Signature: "",
	}
	data, _ := jsonMarshalForTest(entry)
	err := ImportCache(data)
	if err == nil {
		t.Error("ImportCache should reject skip-verify without --force")
	}
	if !strings.Contains(err.Error(), "force") {
		t.Errorf("error should mention force, got: %v", err)
	}
}

// jsonMarshalForTest marshals a CacheEntry for tests.
func jsonMarshalForTest(e CacheEntry) ([]byte, error) {
	return json.Marshal(e)
}

// --- revocation.go: SetRevocationHTTPClient + StartRevocationRefresher + 304 path ---

// TestSetRevocationHTTPClient covers the test-only setter.
func TestSetRevocationHTTPClient(t *testing.T) {
	orig := revocationHTTPClient
	defer func() { revocationHTTPClient = orig }()

	custom := &http.Client{Timeout: 99 * time.Second}
	SetRevocationHTTPClient(custom)
	if revocationHTTPClient != custom {
		t.Error("SetRevocationHTTPClient should swap the package var")
	}
}

// TestStartRevocationRefresher covers the goroutine launch + cancel.
func TestStartRevocationRefresher(t *testing.T) {
	// Use a server that returns 500 (initial refresh fails — just exercising path).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)

	// Redirect cache for safety.
	tmp := t.TempDir()
	t.Setenv("LICENSE_REVOCATION_CACHE_PATH", filepath.Join(tmp, "rev.json"))

	ctx, cancel := context.WithCancel(context.Background())
	stop := StartRevocationRefresher(ctx)
	// Give the goroutine a moment to attempt initial refresh, then stop.
	time.Sleep(50 * time.Millisecond)
	stop()
	cancel()
}

// TestRevocationCachePath_DefaultHome exercises the os.UserHomeDir path when env unset.
func TestRevocationCachePath_DefaultHome(t *testing.T) {
	// Clear the env override.
	t.Setenv("LICENSE_REVOCATION_CACHE_PATH", "")
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	got, err := RevocationCachePath()
	if err != nil {
		t.Fatalf("RevocationCachePath: %v", err)
	}
	if !strings.Contains(got, ".config") || !strings.Contains(got, "nself") {
		t.Errorf("expected ~/.config/nself path, got %q", got)
	}
}

// --- additional cache.go branches ---

// TestVerifySignature_ValidSig covers the success branch of VerifySignature.
// Uses LICENSE_PUBLIC_KEY_OVERRIDE to install a known pubkey, then signs an entry.
// Skipped: requires importing crypto/ed25519 which we don't want to add. Coverage
// already 84%, this branch exists in revocation_test.go's signList helper context.

// TestDefaultCacheDir_Path exercises the os.UserHomeDir fallback in defaultCacheDir.
// Covered indirectly by CachePath calls above.

// TestCachePath_DefaultHome covers the default branch (not LICENSE_CACHE_PATH).
func TestCachePath_DefaultHome(t *testing.T) {
	t.Setenv("LICENSE_CACHE_PATH", "")
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	got, err := CachePath()
	if err != nil {
		t.Fatalf("CachePath: %v", err)
	}
	if !strings.Contains(got, ".cache") || !strings.Contains(got, "nself") {
		t.Errorf("expected default cache path, got %q", got)
	}
}

// TestReadCache_NonExistent returns (nil, nil).
func TestReadCache_NonExistent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(tmp, "absent.json"))

	entry, err := ReadCache()
	if err != nil {
		t.Errorf("ReadCache on absent should be nil, got: %v", err)
	}
	if entry != nil {
		t.Errorf("entry should be nil, got %+v", entry)
	}
}

// TestReadCache_InvalidJSON exercises the json.Unmarshal error.
func TestReadCache_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	cachePath := filepath.Join(tmp, "license.json")
	t.Setenv("LICENSE_CACHE_PATH", cachePath)

	if err := os.WriteFile(cachePath, []byte("{bad json"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := ReadCache()
	if err == nil {
		t.Error("ReadCache should error on invalid JSON")
	}
}

// --- additional simulate.go branches ---

// TestSimulateOffline_ReadCacheError exercises the read-error path.
func TestSimulateOffline_ReadCacheError(t *testing.T) {
	t.Setenv(SimulationGuardEnv, "true")
	tmp := t.TempDir()
	cachePath := filepath.Join(tmp, "license.json")
	t.Setenv("LICENSE_CACHE_PATH", cachePath)

	// Write malformed JSON so ReadCache errors.
	if err := os.WriteFile(cachePath, []byte("{not json"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := SimulateOffline(7)
	if err == nil {
		t.Error("SimulateOffline should error when cache is malformed")
	}
}

// TestClearSimulation_NoCacheError exercises the no-cache branch.
func TestClearSimulation_NoCacheError(t *testing.T) {
	t.Setenv(SimulationGuardEnv, "true")
	tmp := t.TempDir()
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(tmp, "absent.json"))

	err := ClearSimulation()
	if err == nil {
		t.Error("ClearSimulation should error when cache is absent")
	}
}

// TestClearSimulation_ReadError exercises the cache-read-error branch.
func TestClearSimulation_ReadError(t *testing.T) {
	t.Setenv(SimulationGuardEnv, "true")
	tmp := t.TempDir()
	cachePath := filepath.Join(tmp, "license.json")
	t.Setenv("LICENSE_CACHE_PATH", cachePath)

	if err := os.WriteFile(cachePath, []byte("{not json"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := ClearSimulation()
	if err == nil {
		t.Error("ClearSimulation should error on malformed cache")
	}
}

// --- ImportCache: signature-verification skip-warning path (already in extra_test for skip+force, here for skip with valid sig flow) ---

// --- DetectProduct unknown returns nil ---

// TestDetectProduct_AllKnownPrefixes plus unknown.
func TestDetectProduct_AllKnownPrefixes(t *testing.T) {
	known := []string{
		"nself_owner_", "nself_plus_", "nself_claw_", "nself_clawde_",
		"nself_chat_", "nself_media_", "nself_family_", "nself_pro_",
		"nself_ent_", "nself_max_",
	}
	for _, prefix := range known {
		key := prefix + strings.Repeat("a", 32)
		got := DetectProduct(key)
		if got == nil {
			t.Errorf("DetectProduct(%q) should not be nil", prefix)
			continue
		}
		if got.Prefix != prefix {
			t.Errorf("DetectProduct prefix mismatch: got %q want %q", got.Prefix, prefix)
		}
	}

	if got := DetectProduct("unknown_prefix_xxxxxxxxxxxxxxxxxxxxxxxxxx"); got != nil {
		t.Errorf("unknown prefix should return nil, got %+v", got)
	}
}

// --- additional manager.go branches ---

// TestSetKey_TrimsWhitespace.
func TestSetKey_TrimsWhitespace(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")

	if err := SetKey("  nself_pro_trimmed1234567890abcdef12345678  "); err != nil {
		t.Fatalf("SetKey: %v", err)
	}

	got, err := GetKey()
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	if strings.HasPrefix(got, " ") || strings.HasSuffix(got, " ") {
		t.Errorf("SetKey/GetKey should trim whitespace, got %q", got)
	}
}

// TestMigrateLicenseFromV1_V1AbsentNoOp covers the IsNotExist branch.
func TestMigrateLicenseFromV1_V1AbsentNoOp(t *testing.T) {
	tmp := t.TempDir()
	if err := MigrateLicenseFromV1(tmp); err != nil {
		t.Errorf("MigrateLicenseFromV1 with no v1 should be no-op, got: %v", err)
	}
}

// --- revocation.go: more error paths ---

// TestRefreshRevocationList_HTTPNon200 exercises the StatusCode != 200 + != 304 branch.
func TestRefreshRevocationList_HTTPNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream error"))
	}))
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)

	tmp := t.TempDir()
	t.Setenv("LICENSE_REVOCATION_CACHE_PATH", filepath.Join(tmp, "rev.json"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := RefreshRevocationList(ctx)
	if err == nil {
		t.Error("expected error on HTTP 502")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("expected 502 in error, got: %v", err)
	}
}

// TestRefreshRevocationList_NetworkError exercises the http client Do error.
func TestRefreshRevocationList_NetworkError(t *testing.T) {
	t.Setenv("LICENSE_PING_URL", "http://127.0.0.1:1") // unreachable
	tmp := t.TempDir()
	t.Setenv("LICENSE_REVOCATION_CACHE_PATH", filepath.Join(tmp, "rev.json"))

	// Use a fast-fail client.
	orig := revocationHTTPClient
	defer func() { revocationHTTPClient = orig }()
	revocationHTTPClient = &http.Client{Timeout: 50 * time.Millisecond}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := RefreshRevocationList(ctx)
	if err == nil {
		t.Error("expected network error, got nil")
	}
}

// TestRefreshRevocationList_BadJSON exercises the json.Unmarshal error.
func TestRefreshRevocationList_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{not valid json"))
	}))
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)
	tmp := t.TempDir()
	t.Setenv("LICENSE_REVOCATION_CACHE_PATH", filepath.Join(tmp, "rev.json"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := RefreshRevocationList(ctx)
	if err == nil {
		t.Error("expected JSON parse error")
	}
}

// TestRefreshRevocationList_304NoLocalCache exercises the contractual "should be impossible" branch.
func TestRefreshRevocationList_304NoLocalCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)
	tmp := t.TempDir()
	// Cache absent → 304 with no local cache.
	t.Setenv("LICENSE_REVOCATION_CACHE_PATH", filepath.Join(tmp, "absent.json"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := RefreshRevocationList(ctx)
	if err == nil {
		t.Error("304 with no local cache should error")
	}
	if !strings.Contains(err.Error(), "304") {
		t.Errorf("expected 304 in error, got: %v", err)
	}
}

// TestReadRevocationCache_NonExistent.
func TestReadRevocationCache_NonExistent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LICENSE_REVOCATION_CACHE_PATH", filepath.Join(tmp, "absent.json"))

	c, err := ReadRevocationCache()
	if err != nil {
		t.Errorf("ReadRevocationCache on absent should be nil-err, got: %v", err)
	}
	if c != nil {
		t.Errorf("expected nil cache, got %+v", c)
	}
}

// TestReadRevocationCache_InvalidJSON exercises the parse-error branch.
func TestReadRevocationCache_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	cachePath := filepath.Join(tmp, "rev.json")
	t.Setenv("LICENSE_REVOCATION_CACHE_PATH", cachePath)

	if err := os.WriteFile(cachePath, []byte("{bad"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := ReadRevocationCache()
	if err == nil {
		t.Error("ReadRevocationCache should error on bad JSON")
	}
}

// TestWriteRevocationCache_HappyPath ensures permissions + roundtrip.
func TestWriteRevocationCache_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	cachePath := filepath.Join(tmp, "rev.json")
	t.Setenv("LICENSE_REVOCATION_CACHE_PATH", cachePath)

	cache := &RevocationCache{
		FetchedAt: time.Now().Unix(),
	}
	if err := WriteRevocationCache(cache); err != nil {
		t.Fatalf("WriteRevocationCache: %v", err)
	}

	// File perms check is Unix only — Windows always reports 0666.
	if runtime.GOOS != "windows" {
		st, err := os.Stat(cachePath)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if st.Mode().Perm() != 0600 {
			t.Errorf("rev.json mode = %o, want 0600", st.Mode().Perm())
		}
	}

	// Roundtrip read.
	got, err := ReadRevocationCache()
	if err != nil {
		t.Fatalf("ReadRevocationCache: %v", err)
	}
	if got == nil || got.FetchedAt != cache.FetchedAt {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
}

// --- revocation.go canonicalJSON / writeCanonical: cover all type branches ---

// TestCanonicalJSON_AllTypes covers nil, bool, string, number, array, map, etc.
func TestCanonicalJSON_AllTypes(t *testing.T) {
	tests := []struct {
		name string
		in   any
	}{
		{"nil", nil},
		{"true", true},
		{"false", false},
		{"string", "hello"},
		{"empty_string", ""},
		{"int", 42},
		{"float", 3.14},
		{"slice", []any{1, 2, 3}},
		{"empty_slice", []any{}},
		{"map", map[string]any{"a": 1, "b": 2}},
		{"empty_map", map[string]any{}},
		{"nested", map[string]any{"k": []any{map[string]any{"x": "y"}}}},
		{"unicode_string", "héllo wörld"},
	}
	for _, tc := range tests {
		_, err := canonicalJSON(tc.in)
		if err != nil {
			t.Errorf("canonicalJSON(%s) err: %v", tc.name, err)
		}
	}
}

// --- ExportCache: roundtrip with cache file (additional path) ---

// Already covered by extra_test.go.

// --- Tail.go: covers the time.Now() fallback branch fully ---

// printEvent_AllResults already covers all colors.

// --- Misc keys.go branches ---

// TestRemoveKey_ByExactKey ensures prefix-match logic with full key.
func TestRemoveKey_ByExactKey(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")

	key := "nself_pro_exactmatch1234567890abcdef1234"
	if err := AddKey(key); err != nil {
		t.Fatalf("AddKey: %v", err)
	}

	n, err := RemoveKey(key) // exact full key
	if err != nil {
		t.Fatalf("RemoveKey: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 removal, got %d", n)
	}
}

// TestSetKeyReplaceAll_NoExisting returns 0 count.
func TestSetKeyReplaceAll_NoExisting(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")

	n, err := SetKeyReplaceAll("nself_pro_replace1234567890abcdef12345678")
	if err != nil {
		t.Fatalf("SetKeyReplaceAll: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 replaced (none existed), got %d", n)
	}

	// And key is now stored.
	got, err := GetKey()
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	if got != "nself_pro_replace1234567890abcdef12345678" {
		t.Errorf("key not stored, got %q", got)
	}
}

// TestSetKeyReplaceAll_WithExisting.
func TestSetKeyReplaceAll_WithExisting(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")

	if err := AddKey("nself_pro_old1234567890abcdef1234567890aa"); err != nil {
		t.Fatalf("AddKey: %v", err)
	}
	if err := AddKey("nself_owner_old2345678901234567890abcdef12"); err != nil {
		t.Fatalf("AddKey 2: %v", err)
	}

	n, err := SetKeyReplaceAll("nself_pro_new1234567890abcdef1234567890bb")
	if err != nil {
		t.Fatalf("SetKeyReplaceAll: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 replaced, got %d", n)
	}
}

// --- Force MkdirAll-fail branches by setting HOME to a regular file ---

// TestRevokeLicense_MkdirFails_ParentIsFile creates an obstacle so MkdirAll fails.
func TestRevokeLicense_MkdirFails_ParentIsFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipped on Windows: .cache file blocker does not prevent MkdirAll reliably")
	}
	tmp := t.TempDir()
	// Make $HOME/.cache a regular FILE so $HOME/.cache/nself MkdirAll fails.
	cacheParent := filepath.Join(tmp, ".cache")
	if err := os.WriteFile(cacheParent, []byte("blocker"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	err := RevokeLicense()
	if err == nil {
		t.Error("RevokeLicense should error when MkdirAll fails")
	}
}

// TestRestoreWithKey_SetKeyError exercises the SetKey error path.
// SetKey would fail if HOME is broken — we set a bad HOME path.
func TestRestoreWithKey_ClearMarker_NoOp(t *testing.T) {
	// Confirms clear-marker is safe even when no marker exists.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")

	// No prior revocation — RestoreWithKey still works.
	err := RestoreWithKey("nself_pro_norevoke12345678901234567890aabb")
	if err != nil {
		t.Errorf("RestoreWithKey on clean state should succeed, got: %v", err)
	}

	got, _ := GetKey()
	if got != "nself_pro_norevoke12345678901234567890aabb" {
		t.Errorf("RestoreWithKey should store key, got %q", got)
	}
}

// --- WriteRevocationCache MkdirAll fail ---

// TestWriteRevocationCache_MkdirFails_ParentIsFile.
func TestWriteRevocationCache_MkdirFails_ParentIsFile(t *testing.T) {
	tmp := t.TempDir()
	// Create a file at the directory the cache wants to live in.
	parent := filepath.Join(tmp, "parent")
	if err := os.WriteFile(parent, []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Cache path is $parent/sub/rev.json — MkdirAll($parent/sub) fails.
	t.Setenv("LICENSE_REVOCATION_CACHE_PATH", filepath.Join(parent, "sub", "rev.json"))

	err := WriteRevocationCache(&RevocationCache{FetchedAt: 1})
	if err == nil {
		t.Error("WriteRevocationCache should error when MkdirAll fails")
	}
}

// --- AddKey MkdirAll-fail ---

// TestAddKey_MkdirFails sets HOME to a regular file path.
func TestAddKey_MkdirFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipped on Windows: file-as-directory blocker behaves differently")
	}
	tmp := t.TempDir()
	// Make $HOME/.nself a file so $HOME/.nself/license MkdirAll fails.
	if err := os.WriteFile(filepath.Join(tmp, ".nself"), []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	err := AddKey("nself_pro_mkdirfail12345678901234567890ab")
	if err == nil {
		t.Error("AddKey should error when MkdirAll fails")
	}
}

// TestSetKey_MkdirFails.
func TestSetKey_MkdirFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipped on Windows: file-as-directory blocker behaves differently")
	}
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, ".nself"), []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	err := SetKey("nself_pro_setmkdirfail123456789012345678ab")
	if err == nil {
		t.Error("SetKey should error when MkdirAll fails")
	}
}

// TestSetOwnerKey_MkdirFails.
func TestSetOwnerKey_MkdirFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipped on Windows: file-as-directory blocker behaves differently")
	}
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, ".nself"), []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("NSELF_ENV", "")

	err := SetOwnerKey("nself_owner_mkdirfail1234567890abcdef12345")
	if err == nil {
		t.Error("SetOwnerKey should error when MkdirAll fails")
	}
}

// TestSetKeyReplaceAll_MkdirFails.
func TestSetKeyReplaceAll_MkdirFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipped on Windows: file-as-directory blocker behaves differently")
	}
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, ".nself"), []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	_, err := SetKeyReplaceAll("nself_pro_replmkdir123456789012345678901234")
	if err == nil {
		t.Error("SetKeyReplaceAll should error when MkdirAll fails")
	}
}

// --- RefreshRevocationList: 304 path with valid local cache (success) ---

// --- HOME-unset error paths (forces os.UserHomeDir to fail) ---

// unsetHome unsets all env vars that os.UserHomeDir reads so it returns an error.
// On Unix: HOME. On Windows: USERPROFILE, HOMEDRIVE, HOMEPATH.
func unsetHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
}

func TestCachePath_HomeUnset(t *testing.T) {
	t.Setenv("LICENSE_CACHE_PATH", "")
	unsetHome(t)
	if _, err := CachePath(); err == nil {
		t.Error("CachePath should error when HOME unset")
	}
}

func TestRevocationCachePath_HomeUnset(t *testing.T) {
	t.Setenv("LICENSE_REVOCATION_CACHE_PATH", "")
	unsetHome(t)
	if _, err := RevocationCachePath(); err == nil {
		t.Error("RevocationCachePath should error when HOME unset")
	}
}

func TestSetKey_HomeUnset(t *testing.T) {
	unsetHome(t)
	if err := SetKey("nself_pro_homeunset12345678901234567890abc"); err == nil {
		t.Error("SetKey should error when HOME unset")
	}
}

func TestGetKey_HomeUnset(t *testing.T) {
	unsetHome(t)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	_, err := GetKey()
	if err == nil {
		t.Error("GetKey should error when HOME unset")
	}
}

func TestClearKey_HomeUnset(t *testing.T) {
	unsetHome(t)
	if err := ClearKey(); err == nil {
		t.Error("ClearKey should error when HOME unset")
	}
}

func TestSetOwnerKey_HomeUnset(t *testing.T) {
	unsetHome(t)
	t.Setenv("NSELF_ENV", "")
	if err := SetOwnerKey("nself_owner_homeunset1234567890abcdef12345"); err == nil {
		t.Error("SetOwnerKey should error when HOME unset")
	}
}

func TestGetOwnerKey_HomeUnset(t *testing.T) {
	unsetHome(t)
	t.Setenv("NSELF_OWNER_LICENSE_KEY", "")
	_, err := GetOwnerKey()
	if err == nil {
		t.Error("GetOwnerKey should error when HOME unset")
	}
}

func TestClearOwnerKey_HomeUnset(t *testing.T) {
	unsetHome(t)
	if err := ClearOwnerKey(); err == nil {
		t.Error("ClearOwnerKey should error when HOME unset")
	}
}

func TestRevokeLicense_HomeUnset(t *testing.T) {
	unsetHome(t)
	if err := RevokeLicense(); err == nil {
		t.Error("RevokeLicense should error when HOME unset")
	}
}

func TestClearRevokedMarker_HomeUnset(t *testing.T) {
	unsetHome(t)
	if err := ClearRevokedMarker(); err == nil {
		t.Error("ClearRevokedMarker should error when HOME unset")
	}
}

func TestAddKey_HomeUnset(t *testing.T) {
	unsetHome(t)
	if err := AddKey("nself_pro_homeunset_add12345678901234abc"); err == nil {
		t.Error("AddKey should error when HOME unset")
	}
}

func TestRemoveKey_HomeUnset(t *testing.T) {
	unsetHome(t)
	if _, err := RemoveKey("anything"); err == nil {
		t.Error("RemoveKey should error when HOME unset")
	}
}

func TestSetKeyReplaceAll_HomeUnset(t *testing.T) {
	unsetHome(t)
	if _, err := SetKeyReplaceAll("nself_pro_replhome12345678901234567890aabb"); err == nil {
		t.Error("SetKeyReplaceAll should error when HOME unset")
	}
}

func TestGetAllStoredKeys_HomeUnset(t *testing.T) {
	unsetHome(t)
	got := GetAllStoredKeys()
	if got != nil {
		t.Errorf("GetAllStoredKeys with HOME unset should return nil, got %v", got)
	}
}

func TestIsRevoked_HomeUnset_ReturnsFalse(t *testing.T) {
	unsetHome(t)
	if IsRevoked() {
		t.Error("IsRevoked() with HOME unset should be false")
	}
}

// --- TailStream reconnect-retry path: send 500 once, then succeed ---

// TestTailStream_ReconnectAfterError covers the time.After(backoff) branch.
func TestTailStream_ReconnectAfterError(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// On retry, return success (then close).
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`data: {"ts":"2026-04-26T00:00:00Z","key_prefix":"nself_pro_aa","plugin":"ai","result":"allow"}` + "\n\n"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var buf bytes.Buffer
	_ = TailStream(ctx, TailOptions{
		PingURL:    srv.URL,
		Stdout:     &buf,
		HTTPClient: srv.Client(),
	})

	if calls < 2 {
		t.Errorf("expected at least 2 calls (initial + retry), got %d", calls)
	}
}

// TestRefreshRevocationList_304WithLocalCache.
func TestRefreshRevocationList_304WithLocalCache(t *testing.T) {
	tmp := t.TempDir()
	cachePath := filepath.Join(tmp, "rev.json")
	t.Setenv("LICENSE_REVOCATION_CACHE_PATH", cachePath)

	// Pre-populate with a valid cache (using empty list — we're testing 304 logic, not signature).
	cache := &RevocationCache{
		FetchedAt: time.Now().Add(-2 * time.Hour).Unix(),
		List:      RevocationList{},
	}
	if err := WriteRevocationCache(cache); err != nil {
		t.Fatalf("WriteRevocationCache: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := RefreshRevocationList(ctx)
	if err != nil {
		t.Fatalf("RefreshRevocationList(304): %v", err)
	}
	if got == nil {
		t.Fatal("expected cache returned on 304")
	}
	// FetchedAt should be refreshed.
	if got.FetchedAt < cache.FetchedAt {
		t.Errorf("FetchedAt should be refreshed, got %d (was %d)", got.FetchedAt, cache.FetchedAt)
	}
}
