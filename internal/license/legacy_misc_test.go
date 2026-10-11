package license

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// TestClearAllKeyFilesRange pins that key and key.2 through key.10 are removed and key.11 is left alone (D-0226).
func TestClearAllKeyFilesRange(t *testing.T) {
	dir := t.TempDir()
	names := []string{keyFile}
	for i := 2; i <= 11; i++ {
		names = append(names, fmt.Sprintf("%s.%d", keyFile, i))
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("k"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := clearAllKeyFiles(dir); err != nil {
		t.Fatalf("clearAllKeyFiles: %v", err)
	}
	for _, n := range names {
		_, err := os.Stat(filepath.Join(dir, n))
		kept := err == nil
		if want := n == keyFile+".11"; kept != want {
			t.Errorf("%s: kept=%v, want kept=%v", n, kept, want)
		}
	}
}

// TestBundleReplyBoundIssuedAtZero pins that issued_at 0 is refused even when expires_at is small enough to fit the window (D-0226).
func TestBundleReplyBoundIssuedAtZero(t *testing.T) {
	now := time.Unix(100, 0)
	bound := func(issued, expires int64) error {
		return bundleReplyBound(&bundleValidateResponse{Valid: true, Bundle: "chat", KeyHash: HashKey(testLicenseKey),
			IssuedAt: issued, ExpiresAt: expires}, testLicenseKey, "chat", now)
	}
	cases := []struct {
		name            string
		issued, expires int64
		ok              bool
	}{
		{"issued_at 0, expires 3600", 0, 3600, false},
		{"issued_at 1, expires 3600", 1, 3600, true},
		{"issued_at -1, expires 3600", -1, 3600, false},
		{"exactly 24 h long", 1, 1 + replyMaxWindowSec, true},
		{"24 h and 1 s long", 1, 2 + replyMaxWindowSec, false},
		{"empty window", 50, 50, false},
		{"one second window", 50, 101, true},
	}
	for _, c := range cases {
		if err := bound(c.issued, c.expires); (err == nil) != c.ok {
			t.Errorf("%s: err=%v, want ok=%v", c.name, err, c.ok)
		}
	}
}

// TestSimulateOfflineZeroDays pins that 0 days is allowed (valid state) and -1 is refused (D-0226).
func TestSimulateOfflineZeroDays(t *testing.T) {
	compattest.Set(t, false)
	t.Setenv(SimulationGuardEnv, "true")
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(t.TempDir(), "license.json"))
	seedCache(t, &CacheEntry{KeyHash: "h", Tier: "pro", FetchedAt: time.Now().Add(-5 * exactDay).Unix(),
		ExpiresAt: time.Now().Add(365 * exactDay).Unix()})

	res, err := SimulateOffline(0)
	if err != nil {
		t.Fatalf("SimulateOffline(0): %v", err)
	}
	if res.State != GraceValid || !res.WriteAllowed {
		t.Errorf("SimulateOffline(0): got %+v, want a valid state", res)
	}
	if res, err := SimulateOffline(-1); err == nil || res != nil || err.Error() != "days must be non-negative, got -1" {
		t.Errorf("SimulateOffline(-1): got (%v, %v)", res, err)
	}
}
