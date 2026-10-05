// Package license — supplemental_test.go covers gaps not addressed by
// validator_test.go or checker_test.go:
//
//   - realClock.Now()        (0%  → 100%)
//   - tryRemote paths        (56.8% → ~90%): 403, 4xx non-401/403,
//     200+invalid JSON, missing sig header
//   - printEvent             (54.5% → ~100%): all three result branches
//   - DeleteCache error path (66.7% → 100%)
//   - ReadCache corrupt JSON (83.3% → 90%+)
//   - init path              (50% — hard; tested indirectly via key override)
package license

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// realClock
// ============================================================================

// TestRealClock_Now confirms realClock.Now returns a current wall-clock time.
// Tolerance: 5 seconds (avoids flake on heavily loaded CI).
func TestRealClock_Now(t *testing.T) {
	before := time.Now()
	got := realClock{}.Now()
	after := time.Now()

	if got.Before(before.Add(-5*time.Second)) || got.After(after.Add(5*time.Second)) {
		t.Errorf("realClock.Now() = %v, want between %v and %v", got, before, after)
	}
}

// ============================================================================
// tryRemote — uncovered paths
// ============================================================================

// newTryRemoteOpts returns ValidatorOptions pointing at a test server URL,
// with SkipSignatureVerify=true so 200 responses parse without a real signature.
func newTryRemoteOpts(url string) *ValidatorOptions {
	return &ValidatorOptions{
		PingURL:             url,
		SkipSignatureVerify: true,
		WarnOnce:            func(string) {},
	}
}

// TestTryRemote_403_AuthFail confirms a 403 yields remoteAuthFail.
func TestTryRemote_403_AuthFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, kind, _ := tryRemote(context.Background(), "nself_pro_testkey1234567890AAAAAAAAAA", newTryRemoteOpts(srv.URL))
	if kind != remoteAuthFail {
		t.Errorf("403: kind = %v, want remoteAuthFail", kind)
	}
}

// TestTryRemote_404_AuthFail confirms a 404 (other 4xx) yields remoteAuthFail
// (conservative posture: not fail-open on unexpected client errors).
func TestTryRemote_404_AuthFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, kind, err := tryRemote(context.Background(), "nself_pro_testkey1234567890AAAAAAAAAA", newTryRemoteOpts(srv.URL))
	if kind != remoteAuthFail {
		t.Errorf("404: kind = %v, want remoteAuthFail; err = %v", kind, err)
	}
}

// TestTryRemote_200_InvalidJSON confirms a 200 with unparseable body yields
// remoteTransientFail (server bug treated as transient, not auth).
func TestTryRemote_200_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "not json at all")
	}))
	defer srv.Close()

	_, kind, err := tryRemote(context.Background(), "nself_pro_testkey1234567890AAAAAAAAAA", newTryRemoteOpts(srv.URL))
	if kind != remoteTransientFail {
		t.Errorf("200+bad JSON: kind = %v, want remoteTransientFail; err = %v", kind, err)
	}
}

// nonZeroPubKeyHex returns a valid 32-byte non-zero Ed25519 public key as hex.
// Using a fixed deterministic key so IsZeroPubKey() returns false — which enables
// the production signature-verification branch in tryRemote.
func nonZeroPubKeyHex() string {
	key := make([]byte, 32)
	key[0] = 0xAA
	key[1] = 0xBB
	return hex.EncodeToString(key)
}

// TestTryRemote_200_WithMissingSignatureHeader confirms that when
// SkipSignatureVerify=false (production path) and a public key is injected via
// LICENSE_PUBLIC_KEY_OVERRIDE, a missing X-NSelf-License-Sig header causes
// remoteTransientFail.
func TestTryRemote_200_MissingSignatureHeader(t *testing.T) {
	// Set override env so IsZeroPubKey() returns false → verify path runs.
	useTestKey(t, nonZeroPubKeyHex())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"valid":true,"tier":"pro"}`)
		// No X-NSelf-License-Sig header set.
	}))
	defer srv.Close()

	opts := &ValidatorOptions{
		PingURL:             srv.URL,
		SkipSignatureVerify: false, // real verification
		WarnOnce:            func(string) {},
	}
	_, kind, err := tryRemote(context.Background(), "nself_pro_testkey1234567890AAAAAAAAAA", opts)
	if kind != remoteTransientFail {
		t.Errorf("missing sig header: kind = %v, want remoteTransientFail; err = %v", kind, err)
	}
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Errorf("expected signature error, got %v", err)
	}
}

// TestTryRemote_200_MalformedSignatureHeader confirms a malformed hex signature
// header triggers remoteTransientFail.
func TestTryRemote_200_MalformedSignatureHeader(t *testing.T) {
	useTestKey(t, nonZeroPubKeyHex())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-NSelf-License-Sig", "ZZZnotvalidhex!!!")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"valid":true}`)
	}))
	defer srv.Close()

	opts := &ValidatorOptions{
		PingURL:             srv.URL,
		SkipSignatureVerify: false,
		WarnOnce:            func(string) {},
	}
	_, kind, _ := tryRemote(context.Background(), "nself_pro_testkey1234567890AAAAAAAAAA", opts)
	if kind != remoteTransientFail {
		t.Errorf("malformed hex sig: kind = %v, want remoteTransientFail", kind)
	}
}

// TestTryRemote_200_InvalidSignatureBytes confirms a correctly-hex-encoded but
// cryptographically wrong signature triggers remoteTransientFail.
func TestTryRemote_200_InvalidSignatureBytes(t *testing.T) {
	useTestKey(t, nonZeroPubKeyHex())

	badSig := make([]byte, 64) // 64 zero bytes — valid hex, wrong signature
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-NSelf-License-Sig", hex.EncodeToString(badSig))
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"valid":true}`)
	}))
	defer srv.Close()

	opts := &ValidatorOptions{
		PingURL:             srv.URL,
		SkipSignatureVerify: false,
		WarnOnce:            func(string) {},
	}
	_, kind, _ := tryRemote(context.Background(), "nself_pro_testkey1234567890AAAAAAAAAA", opts)
	if kind != remoteTransientFail {
		t.Errorf("invalid sig bytes: kind = %v, want remoteTransientFail", kind)
	}
}

// TestTryRemote_TransportError confirms a TCP-level failure returns
// remoteTransientFail (existing path — belt-and-suspenders coverage).
func TestTryRemote_TransportError(t *testing.T) {
	// Use a hijack server so the connection resets instantly on all platforms.
	// Bare 127.0.0.1:19998 DROPs on Windows Firewall and hangs 30s until the
	// http.Client timeout fires.
	refuseSrv := newRefusingServer(t)
	opts := &ValidatorOptions{
		PingURL:             refuseSrv.URL,
		SkipSignatureVerify: true,
		WarnOnce:            func(string) {},
	}
	_, kind, err := tryRemote(context.Background(), "nself_pro_testkey1234567890AAAAAAAAAA", opts)
	if kind != remoteTransientFail {
		t.Errorf("transport error: kind = %v, want remoteTransientFail; err = %v", kind, err)
	}
}

// ============================================================================
// printEvent
// ============================================================================

// TestPrintEvent_AllResultBranches checks that all three color branches of
// printEvent produce output containing the result string.
func TestPrintEvent_AllResultBranches(t *testing.T) {
	fixed := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		result  string
		wantStr string
	}{
		{result: "allow", wantStr: "ALLOW"},
		{result: "deny", wantStr: "DENY"},
		{result: "rate_limit", wantStr: "RATE_LIMIT"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.result, func(t *testing.T) {
			ev := LicenseEvent{
				Timestamp: fixed,
				KeyPrefix: "nself_pro_",
				Plugin:    "ai",
				Result:    tc.result,
			}
			var buf bytes.Buffer
			printEvent(&buf, ev)
			out := buf.String()
			if !strings.Contains(out, tc.wantStr) {
				t.Errorf("printEvent(%q): output %q does not contain %q", tc.result, out, tc.wantStr)
			}
		})
	}
}

// TestPrintEvent_UnknownResultNoColor confirms an unrecognized result string
// gets no color escape and still produces non-empty output.
func TestPrintEvent_UnknownResultNoColor(t *testing.T) {
	ev := LicenseEvent{
		Timestamp: time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC),
		KeyPrefix: "pfx",
		Plugin:    "mux",
		Result:    "unknown_result",
	}
	var buf bytes.Buffer
	printEvent(&buf, ev)
	out := buf.String()
	if strings.Contains(out, "\033[") {
		t.Errorf("unexpected color escape in non-terminal output: %q", out)
	}
	if len(out) == 0 {
		t.Error("empty output for unknown result")
	}
}

// ============================================================================
// DeleteCache — error path
// ============================================================================

// TestDeleteCache_PermissionDenied verifies that DeleteCache returns an error
// when the cache file exists but os.Remove fails (e.g., the directory is
// read-only). This covers the non-IsNotExist error branch.
func TestDeleteCache_PermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipped on Windows: chmod does not restrict file removal")
	}
	if os.Getuid() == 0 {
		t.Skip("test requires non-root: root can remove files in read-only dirs")
	}

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "license.json")
	t.Setenv("LICENSE_CACHE_PATH", cachePath)

	// Write the cache file.
	entry := &CacheEntry{
		KeyHash:   HashKey("some-key"),
		Tier:      "pro",
		FetchedAt: time.Now().Unix(),
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	if err := WriteCache(entry); err != nil {
		t.Fatalf("setup: WriteCache: %v", err)
	}

	// Make the parent directory read-only so Remove will fail.
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatalf("setup: chmod dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })

	err := DeleteCache()
	if err == nil {
		t.Error("DeleteCache on read-only dir: expected error, got nil")
	}
}

// TestDeleteCache_FileNotExist confirms DeleteCache succeeds silently when
// the cache file doesn't exist (IsNotExist is ignored).
func TestDeleteCache_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(dir, "nonexistent.json"))

	if err := DeleteCache(); err != nil {
		t.Errorf("DeleteCache on missing file: expected nil, got %v", err)
	}
}

// ============================================================================
// ReadCache — corrupt JSON path
// ============================================================================

// TestReadCache_CorruptJSON confirms ReadCache returns a non-nil error when the
// file exists but contains invalid JSON.
func TestReadCache_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "license.json")
	t.Setenv("LICENSE_CACHE_PATH", cachePath)

	if err := os.WriteFile(cachePath, []byte("{corrupt json!!"), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	entry, err := ReadCache()
	if err == nil {
		t.Error("ReadCache corrupt JSON: expected error, got nil")
	}
	if entry != nil {
		t.Error("ReadCache corrupt JSON: expected nil entry, got non-nil")
	}
}

// ============================================================================
// init — indirect coverage via LICENSE_PUBLIC_KEY_OVERRIDE
// ============================================================================

// TestGetPublicKeys_EmptyOverride confirms an empty override env var returns
// the bundled keys without panic.
func TestGetPublicKeys_EmptyOverride(t *testing.T) {
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", "")
	keys := GetPublicKeys()
	if len(keys) == 0 {
		t.Error("expected at least 1 bundled key when override is empty")
	}
}

// ============================================================================
// Validate — remote 403 path (auth fail, not fail-open)
// ============================================================================

// TestValidate_Remote403_FailClosed confirms a 403 is treated as auth failure,
// never falling through to the fail-open path even with a fresh cache.
func TestValidate_Remote403_FailClosed(t *testing.T) {
	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "license.json")
	t.Setenv("LICENSE_CACHE_PATH", cachePath)

	key := "nself_pro_testkey1234567890abcdef12345"
	// Fresh cache present.
	entry := &CacheEntry{
		KeyHash:        HashKey(key),
		Tier:           "pro",
		PluginsAllowed: []string{"ai"},
		FetchedAt:      now.Add(-1 * time.Hour).Unix(),
		ExpiresAt:      now.Add(30 * 24 * time.Hour).Unix(),
	}
	if err := WriteCache(entry); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	var warned []string
	var mu sync.Mutex
	warnFn := func(m string) {
		mu.Lock()
		warned = append(warned, m)
		mu.Unlock()
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

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
	if res.Status != StatusFailClosed {
		t.Errorf("403: status = %q, want %q (auth fail must not fail-open)", res.Status, StatusFailClosed)
	}
	if res.CanProceed {
		t.Error("403: CanProceed = true, want false")
	}
}

// TestValidate_RemoteValid_ExpiredBody confirms that a 200 response with
// valid=false and "expired" in the reason yields StatusExpired.
func TestValidate_RemoteValid_ExpiredBody(t *testing.T) {
	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)

	dir := t.TempDir()
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(dir, "license.json"))

	key := "nself_pro_testkey1234567890abcdef12345"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"valid":false,"reason":"license expired 7 days ago"}`)
	}))
	defer srv.Close()

	warnFn := func(string) {}
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
	if res.Status != StatusExpired {
		t.Errorf("status = %q, want %q (expired reason in body)", res.Status, StatusExpired)
	}
	if res.CanProceed {
		t.Error("CanProceed = true, want false for expired")
	}
}
