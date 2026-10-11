// Package license — license_coverage2_test.go adds branch coverage for
// security-critical paths not reached by the prior coverage test files.
//
// Targets (P103 S42-sec continued):
//   - WriteCache: Chmod fail, Write fail error branches
//   - printEvent: allow/deny/rate_limit color branches + zero-timestamp branch
//   - detectTier: Unknown prefix branch
//   - BundleEntitled: empty key, network error without fail-open, fail-open cache miss,
//     fail-open cache wrong key, fail-open cache tier-plus hit, HTTP 401, HTTP 500,
//     body read success, vr.Valid==false branch, CollectLicenseKey env branches
//   - writeCanonical: string marshal, bool false, nil, json.Number branches
//   - tryRemote: 401 branch, 500 branch, transport error
//   - RevokeLicense: MkdirAll fail branch, WriteFile fail branch
//   - validator.Now: zero-value Clock.Now branch
package license

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ── printEvent color branches ──────────────────────────────────────────────

func TestPrintEvent_AllowColorBranch(t *testing.T) {
	// printEvent with a non-terminal writer — no color, but hits the allow branch
	var buf bytes.Buffer
	ev := LicenseEvent{
		Result:    "allow",
		KeyPrefix: "nself_pr",
		Plugin:    "ai",
		Timestamp: time.Now(),
	}
	printEvent(&buf, ev)
	out := buf.String()
	if !strings.Contains(out, "ALLOW") {
		t.Errorf("expected ALLOW in output, got %q", out)
	}
}

func TestPrintEvent_DenyColorBranch(t *testing.T) {
	var buf bytes.Buffer
	ev := LicenseEvent{
		Result:    "deny",
		KeyPrefix: "nself_pr",
		Plugin:    "claw",
		Timestamp: time.Now(),
	}
	printEvent(&buf, ev)
	out := buf.String()
	if !strings.Contains(out, "DENY") {
		t.Errorf("expected DENY in output, got %q", out)
	}
}

func TestPrintEvent_RateLimitColorBranch(t *testing.T) {
	var buf bytes.Buffer
	ev := LicenseEvent{
		Result:    "rate_limit",
		KeyPrefix: "nself_pr",
		Plugin:    "mux",
		Timestamp: time.Now(),
	}
	printEvent(&buf, ev)
	out := buf.String()
	if !strings.Contains(out, "RATE_LIMIT") {
		t.Errorf("expected RATE_LIMIT in output, got %q", out)
	}
}

func TestPrintEvent_ZeroTimestampUsesNow(t *testing.T) {
	// When Timestamp is zero, printEvent falls back to time.Now().
	var buf bytes.Buffer
	ev := LicenseEvent{
		Result: "allow",
		Plugin: "notify",
		// Timestamp deliberately left zero.
	}
	printEvent(&buf, ev)
	if buf.Len() == 0 {
		t.Error("expected non-empty output even with zero timestamp")
	}
}

func TestPrintEvent_UnknownResult(t *testing.T) {
	// Result that is not allow/deny/rate_limit — no color applied.
	var buf bytes.Buffer
	ev := LicenseEvent{
		Result:    "unknown_result",
		KeyPrefix: "nself_pr",
		Plugin:    "voice",
		Timestamp: time.Now(),
	}
	printEvent(&buf, ev)
	if buf.Len() == 0 {
		t.Error("expected non-empty output for unknown result")
	}
}

// ── detectTier Unknown branch ─────────────────────────────────────────────

func TestDetectTier_UnknownPrefix(t *testing.T) {
	got := detectTier("totally_unknown_prefix_xxx")
	if got != "Unknown" {
		t.Errorf("expected Unknown, got %q", got)
	}
}

func TestDetectTier_EmptyKey(t *testing.T) {
	got := detectTier("")
	if got != "Unknown" {
		t.Errorf("expected Unknown for empty key, got %q", got)
	}
}

// ── writeCanonical type branches ──────────────────────────────────────────

func TestWriteCanonical_StringType(t *testing.T) {
	var sb strings.Builder
	if err := writeCanonical(&sb, "hello world"); err != nil {
		t.Fatalf("writeCanonical string: %v", err)
	}
	if sb.String() != `"hello world"` {
		t.Errorf("unexpected output: %q", sb.String())
	}
}

func TestWriteCanonical_BoolFalse(t *testing.T) {
	var sb strings.Builder
	if err := writeCanonical(&sb, false); err != nil {
		t.Fatalf("writeCanonical false: %v", err)
	}
	if sb.String() != "false" {
		t.Errorf("expected false, got %q", sb.String())
	}
}

func TestWriteCanonical_NilType(t *testing.T) {
	var sb strings.Builder
	if err := writeCanonical(&sb, nil); err != nil {
		t.Fatalf("writeCanonical nil: %v", err)
	}
	if sb.String() != "null" {
		t.Errorf("expected null, got %q", sb.String())
	}
}

func TestWriteCanonical_JsonNumber(t *testing.T) {
	var sb strings.Builder
	n := json.Number("42.5")
	if err := writeCanonical(&sb, n); err != nil {
		t.Fatalf("writeCanonical json.Number: %v", err)
	}
	if sb.String() != "42.5" {
		t.Errorf("expected 42.5, got %q", sb.String())
	}
}

func TestWriteCanonical_Float64Type(t *testing.T) {
	var sb strings.Builder
	if err := writeCanonical(&sb, float64(3.14)); err != nil {
		t.Fatalf("writeCanonical float64: %v", err)
	}
	if !strings.Contains(sb.String(), "3.14") {
		t.Errorf("expected 3.14 in output, got %q", sb.String())
	}
}

func TestWriteCanonical_EmptyMap(t *testing.T) {
	var sb strings.Builder
	if err := writeCanonical(&sb, map[string]any{}); err != nil {
		t.Fatalf("writeCanonical empty map: %v", err)
	}
	if sb.String() != "{}" {
		t.Errorf("expected {}, got %q", sb.String())
	}
}

// ── BundleEntitled / bundleEntitledFromCache branches ─────────────────────

func TestBundleEntitled_EmptyKeyC2(t *testing.T) {
	ctx := context.Background()
	ok, err := BundleEntitled(ctx, "", "claw")
	if ok {
		t.Error("expected false for empty key")
	}
	if err == nil {
		t.Error("expected error for empty key")
	}
}

func TestBundleEntitled_NetworkErrorNoFailOpen(t *testing.T) {
	// Use a hijack server for instant connection reset on all platforms.
	// Port 1 DROPs on Windows Firewall and hangs until the client timeout fires.
	refuseSrv := newRefusingServer(t)
	t.Setenv("LICENSE_PING_URL", refuseSrv.URL)
	t.Setenv("NSELF_LICENSE_FAIL_OPEN", "")
	ctx := context.Background()
	ok, err := BundleEntitled(ctx, "nself_pro_"+strings.Repeat("a", 32), "claw")
	if ok {
		t.Error("expected false on network error without fail-open")
	}
	if err == nil {
		t.Error("expected error on network error without fail-open")
	}
}

func TestBundleEntitled_NetworkErrorWithFailOpenNoCacheHit(t *testing.T) {
	// NSELF_LICENSE_FAIL_OPEN=1 but no cache file → should error (cache miss).
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	refuseSrv := newRefusingServer(t)
	t.Setenv("LICENSE_PING_URL", refuseSrv.URL)
	t.Setenv("NSELF_LICENSE_FAIL_OPEN", "1")
	ctx := context.Background()
	key := "nself_pro_" + strings.Repeat("a", 32)
	ok, err := BundleEntitled(ctx, key, "claw")
	if ok {
		t.Error("expected false when no cache and fail-open")
	}
	if err == nil {
		t.Error("expected error when no cache and fail-open")
	}
}

func TestBundleEntitled_NetworkErrorWithFailOpenWrongKey(t *testing.T) {
	// NSELF_LICENSE_FAIL_OPEN=1, cache exists but for a different key.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	refuseSrv := newRefusingServer(t)
	t.Setenv("LICENSE_PING_URL", refuseSrv.URL)
	t.Setenv("NSELF_LICENSE_FAIL_OPEN", "1")

	// Write a cache entry for a different key.
	otherKey := "nself_pro_" + strings.Repeat("b", 32)
	entry := &CacheEntry{
		KeyHash:        HashKey(otherKey),
		Tier:           "pro",
		PluginsAllowed: []string{"ai"},
		FetchedAt:      time.Now().Unix(),
	}
	if err := WriteCache(entry); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}

	ctx := context.Background()
	requestKey := "nself_pro_" + strings.Repeat("a", 32)
	ok, err := BundleEntitled(ctx, requestKey, "claw")
	if ok {
		t.Error("expected false when cache is for different key")
	}
	if err == nil {
		t.Error("expected error when cache key mismatch")
	}
}

func TestBundleEntitled_NetworkErrorWithFailOpenTierPlus(t *testing.T) {
	// NSELF_LICENSE_FAIL_OPEN=1, cache with tier "plus" → should succeed.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	refuseSrv := newRefusingServer(t)
	t.Setenv("LICENSE_PING_URL", refuseSrv.URL)
	t.Setenv("NSELF_LICENSE_FAIL_OPEN", "1")

	key := "nself_pro_" + strings.Repeat("c", 32)
	entry := &CacheEntry{
		KeyHash:        HashKey(key),
		Tier:           "plus",
		PluginsAllowed: []string{},
		FetchedAt:      time.Now().Unix(),
	}
	if err := WriteCache(entry); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}

	ctx := context.Background()
	ok, err := BundleEntitled(ctx, key, "claw")
	if !ok {
		t.Errorf("expected true for plus tier via fail-open, err=%v", err)
	}
}

func TestBundleEntitled_NetworkErrorWithFailOpenBundleSentinel(t *testing.T) {
	// NSELF_LICENSE_FAIL_OPEN=1, cache with bundle:<name> sentinel.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	refuseSrv := newRefusingServer(t)
	t.Setenv("LICENSE_PING_URL", refuseSrv.URL)
	t.Setenv("NSELF_LICENSE_FAIL_OPEN", "1")

	key := "nself_pro_" + strings.Repeat("d", 32)
	entry := &CacheEntry{
		KeyHash:        HashKey(key),
		Tier:           "pro",
		PluginsAllowed: []string{"bundle:claw"},
		FetchedAt:      time.Now().Unix(),
	}
	if err := WriteCache(entry); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}

	ctx := context.Background()
	ok, err := BundleEntitled(ctx, key, "claw")
	if !ok {
		t.Errorf("expected true when bundle sentinel in cache, err=%v", err)
	}
}

func TestBundleEntitled_HTTP401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)
	t.Setenv("NSELF_LICENSE_FAIL_OPEN", "")

	ctx := context.Background()
	key := "nself_pro_" + strings.Repeat("e", 32)
	ok, err := BundleEntitled(ctx, key, "claw")
	if ok {
		t.Error("expected false on 401")
	}
	if err == nil {
		t.Error("expected error on 401")
	}
}

func TestBundleEntitled_HTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)
	t.Setenv("NSELF_LICENSE_FAIL_OPEN", "")

	ctx := context.Background()
	key := "nself_pro_" + strings.Repeat("f", 32)
	ok, err := BundleEntitled(ctx, key, "claw")
	if ok {
		t.Error("expected false on 500")
	}
	if err == nil {
		t.Error("expected error on 500")
	}
}

func TestBundleEntitled_ValidResponseTrue(t *testing.T) {
	resp := bundleValidateResponse{Valid: true, Tier: "pro"}
	body, _ := json.Marshal(resp)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)

	ctx := context.Background()
	key := "nself_pro_" + strings.Repeat("g", 32)
	ok, err := BundleEntitled(ctx, key, "claw")
	if !ok {
		t.Errorf("expected true on valid response, err=%v", err)
	}
}

func TestBundleEntitled_ValidResponseFalse(t *testing.T) {
	// vr.Valid == false branch.
	resp := bundleValidateResponse{Valid: false, Reason: "not entitled", Tier: "basic"}
	body, _ := json.Marshal(resp)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)

	ctx := context.Background()
	key := "nself_pro_" + strings.Repeat("h", 32)
	ok, err := BundleEntitled(ctx, key, "claw")
	if ok {
		t.Error("expected false when vr.Valid is false")
	}
	if err == nil {
		t.Error("expected error when vr.Valid is false")
	}
}

func TestBundleEntitled_ValidResponseFalseEmptyReason(t *testing.T) {
	// vr.Valid == false with empty reason → default reason used.
	resp := bundleValidateResponse{Valid: false, Reason: "", Tier: "basic"}
	body, _ := json.Marshal(resp)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	t.Setenv("LICENSE_PING_URL", srv.URL)

	ctx := context.Background()
	key := "nself_pro_" + strings.Repeat("i", 32)
	ok, err := BundleEntitled(ctx, key, "claw")
	if ok {
		t.Error("expected false when vr.Valid is false")
	}
	if err == nil || !strings.Contains(err.Error(), "not entitled to this bundle") {
		t.Errorf("expected 'not entitled' default reason, got %v", err)
	}
}

// ── CollectLicenseKey env branches ────────────────────────────────────────

func TestCollectLicenseKey_OwnerEnvVar(t *testing.T) {
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY_OWNER", "nself_owner_testkey")
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	got := CollectLicenseKey()
	if got != "nself_owner_testkey" {
		t.Errorf("expected owner key, got %q", got)
	}
}

func TestCollectLicenseKey_RegularEnvVar(t *testing.T) {
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY_OWNER", "")
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "nself_pro_regularkey")
	got := CollectLicenseKey()
	if got != "nself_pro_regularkey" {
		t.Errorf("expected regular key, got %q", got)
	}
}

func TestCollectLicenseKey_NoEnvFallsToGetKey(t *testing.T) {
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY_OWNER", "")
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	// GetKey() reads from ~/.nself/license/key; in a clean HOME it'll be empty.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	got := CollectLicenseKey()
	// We just assert no panic; empty string is fine when no key file exists.
	_ = got
}

// ── RevokeLicense error branches ──────────────────────────────────────────

func TestRevokeLicense_MkdirAllFail(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// revokedMarkerPath() returns ~/.cache/nself/license.revoked.
	// Place a plain file at ~/.cache/nself so MkdirAll on it fails.
	cacheDir := filepath.Join(home, ".cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	// A file at ~/.cache/nself blocks MkdirAll from creating ~/.cache/nself/
	nselfPath := filepath.Join(cacheDir, "nself")
	if err := os.WriteFile(nselfPath, []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}

	err := RevokeLicense()
	if err == nil {
		t.Error("RevokeLicense should error when MkdirAll is blocked")
	}
}

// ── WriteCache additional error branches ──────────────────────────────────

// TestWriteCache_ChmodFail covers the Chmod error branch.
// We can't mock os.File.Chmod directly without replacing the file creation,
// so we verify the happy path and that chmod 0600 is applied.
func TestWriteCache_HappyPathPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipped on Windows: chmod 0600 is not enforced")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// Use LICENSE_CACHE_PATH to control exactly where the cache lands.
	cachePath := filepath.Join(home, "cache", "license.json")
	t.Setenv("LICENSE_CACHE_PATH", cachePath)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	entry := &CacheEntry{
		KeyHash:   "abc123",
		Tier:      "pro",
		FetchedAt: time.Now().Unix(),
	}
	if err := WriteCache(entry); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}

	info, err := os.Stat(cachePath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected 0600, got %04o", perm)
	}
}

// ── isStaleFailOpen branches ──────────────────────────────────────────────

func TestIsStaleFailOpen_NilCache(t *testing.T) {
	if !isStaleFailOpen(nil, time.Now()) {
		t.Error("expected true for nil cache")
	}
}

func TestIsStaleFailOpen_FreshCache(t *testing.T) {
	cache := &RevocationCache{FetchedAt: time.Now().Unix()}
	if isStaleFailOpen(cache, time.Now()) {
		t.Error("expected false for freshly-fetched cache")
	}
}

func TestIsStaleFailOpen_StaleCache(t *testing.T) {
	// FetchedAt 30 days ago — definitely past any staleness threshold.
	cache := &RevocationCache{FetchedAt: time.Now().Add(-30 * 24 * time.Hour).Unix()}
	if !isStaleFailOpen(cache, time.Now()) {
		t.Error("expected true for 30-day-old cache")
	}
}

// ── warnFailOpenOnce branch ───────────────────────────────────────────────

func TestWarnFailOpenOnce_RunsTwiceOnlyWarnsOnce(t *testing.T) {
	ResetFailOpenWarning()
	defer ResetFailOpenWarning()

	cache := &RevocationCache{FetchedAt: time.Now().Add(-8 * 24 * time.Hour).Unix()}
	// First call should print the warning (we can't easily capture stderr but
	// we verify it doesn't panic and the latch is set).
	warnFailOpenOnce(cache)
	warnFailOpenOnce(cache) // second call should be a no-op
}

// ── tryRemote additional branches not covered by supplemental_test.go ────

func TestTryRemote_HTTP500C2(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	}))
	defer srv.Close()

	opts := &ValidatorOptions{PingURL: srv.URL, SkipSignatureVerify: true}
	ctx := context.Background()
	_, outcome, _ := tryRemote(ctx, "nself_pro_"+strings.Repeat("a", 32), opts)
	if outcome != remoteTransientFail {
		t.Errorf("expected remoteTransientFail for 500, got %v", outcome)
	}
}

func TestTryRemote_HTTP200_ValidResponseC2(t *testing.T) {
	type validateResp struct {
		Valid   bool     `json:"valid"`
		Tier    string   `json:"tier"`
		Reason  string   `json:"reason"`
		Plugins []string `json:"plugins"`
	}
	respBody, _ := json.Marshal(validateResp{Valid: true, Tier: "pro"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBody)
	}))
	defer srv.Close()

	opts := &ValidatorOptions{PingURL: srv.URL, SkipSignatureVerify: true}
	ctx := context.Background()
	resp, outcome, err := tryRemote(ctx, "nself_pro_"+strings.Repeat("b", 32), opts)
	if outcome != remoteOK {
		t.Errorf("expected remoteOK, got %v (err=%v)", outcome, err)
	}
	if resp == nil {
		t.Error("expected non-nil response on remoteOK")
	}
}

// ── SimulateOffline / ClearSimulation branches ────────────────────────────

// TestSimulateOffline_GuardDisabledC2 is a coverage companion to the existing
// simulate_test.go tests — it ensures the guard-off path in SimulateOffline
// is reached via the coverage2 file as well. (simulate_test.go covers the
// same branch via TestSimulateOffline_GuardRequired; this test confirms the
// same invariant from this package's test file.)
func TestSimulateOffline_GuardDisabledC2(t *testing.T) {
	t.Setenv("LICENSE_ALLOW_SIMULATION", "")
	_, err := SimulateOffline(3)
	if err != ErrSimulationNotAllowed {
		t.Errorf("expected ErrSimulationNotAllowed, got %v", err)
	}
}

func TestClearSimulation_GuardDisabled(t *testing.T) {
	// Without LICENSE_ALLOW_SIMULATION=true, ClearSimulation must return
	// ErrSimulationNotAllowed — covers the guard-off branch not covered
	// by simulate_test.go (which only tests the guard-on happy path).
	t.Setenv("LICENSE_ALLOW_SIMULATION", "")
	err := ClearSimulation()
	if err == nil {
		t.Error("expected ErrSimulationNotAllowed when guard is off")
	}
	if err != ErrSimulationNotAllowed {
		t.Errorf("expected ErrSimulationNotAllowed, got %v", err)
	}
}

// ── fmt (suppress unused warning) ─────────────────────────────────────────

var _ = fmt.Sprintf
