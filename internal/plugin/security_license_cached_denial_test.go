package plugin

// Purpose: regression tests for D-0474 — checkLicense must not let a cached-valid
// key override a cached per-plugin entitlement denial.
// Inputs: a throwaway HOME holding the licence cache, and an httptest licence server.
// Outputs: pass/fail only.
// Constraints: no real network; every server is local.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

const deniedTestKey = "nself_pro_deniedcachekey_aaaaaaaaaaaaaaaa"

// deniedCacheFixture points HOME at a temp dir, sets the key, writes a fresh
// cached-valid key entry and a fresh entitlement cache for tier/plugins, and
// returns a counter of licence-server calls.
func deniedCacheFixture(t *testing.T, tier string, plugins []string, serverBody string) *int32 {
	t.Helper()
	home := t.TempDir()
	setTestHome(t, home)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", deniedTestKey)
	for _, v := range []string{"NSELF_LICENSE_KEY_1", "NSELF_LICENSE_KEY_2"} {
		t.Setenv(v, "")
	}
	cacheDir := filepath.Join(home, ".nself", "license")
	writeCacheEntryWithAge(t, cacheDir, deniedTestKey, "valid", time.Minute)
	if err := cacheEntitlements(cacheDir, tier, plugins); err != nil {
		t.Fatal(err)
	}
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(serverBody))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("NSELF_PING_API_URL", srv.URL)
	return &hits
}

// A cached denial for a bundle tier must be confirmed live; the server still
// refuses, so the install is refused (it used to pass on the cached-valid key).
func TestCheckLicenseCachedDenialIsNotOverriddenByCachedKey(t *testing.T) {
	hits := deniedCacheFixture(t, "chat", []string{"chat"}, `{"valid":true,"tier":"chat","plugins":["chat"]}`)
	err := checkLicense(context.Background(), "ai")
	if !errors.Is(err, errs.ErrLicenseTierTooLow) {
		t.Fatalf("checkLicense = %v, want ErrLicenseTierTooLow", err)
	}
	if atomic.LoadInt32(hits) != 1 {
		t.Errorf("licence server calls = %d, want 1 (live confirmation)", atomic.LoadInt32(hits))
	}
}

// If the live check finds the plugin entitled now (key upgraded inside the cache
// TTL), the install passes.
func TestCheckLicenseCachedDenialLiftedByLiveConfirmation(t *testing.T) {
	deniedCacheFixture(t, "chat", []string{"chat"}, `{"valid":true,"tier":"plus","plugins":[]}`)
	if err := checkLicense(context.Background(), "ai"); err != nil {
		t.Fatalf("checkLicense = %v, want nil after live all-access confirmation", err)
	}
}

// With the server unreachable, a cached denial must not be bypassed by the
// offline cache either.
func TestCheckLicenseCachedDenialNoOfflineBypass(t *testing.T) {
	deniedCacheFixture(t, "chat", []string{"chat"}, `{}`)
	t.Setenv("NSELF_PING_API_URL", "http://127.0.0.1:1")
	if err := checkLicense(context.Background(), "ai"); err == nil {
		t.Fatal("checkLicense passed offline despite a cached entitlement denial")
	}
}

// An all-access cached tier (empty plugin list) keeps trusting the cached key
// with no server call.
func TestCheckLicenseAllAccessCacheStillOffline(t *testing.T) {
	hits := deniedCacheFixture(t, "plus", nil, `{}`)
	if err := checkLicense(context.Background(), "ai"); err != nil {
		t.Fatalf("checkLicense = %v, want nil", err)
	}
	if atomic.LoadInt32(hits) != 0 {
		t.Errorf("licence server calls = %d, want 0", atomic.LoadInt32(hits))
	}
}
