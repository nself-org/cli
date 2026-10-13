package bundle

// installer_helpers_licence_test.go — D-0372: the licence pre-flight resolves
// each member through plugin.ResolvePlugin and refuses a licensed member the
// cached entitlements exclude (unless the cached tier is all-access). No
// network: registries are built in memory or served from a seeded cache. Keys
// are obviously fake.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/plugin"
)

func never(context.Context, []string) (bool, error)  { return false, nil }
func always(context.Context, []string) (bool, error) { return true, nil }

func pairRegistry(freeFirst bool) *plugin.Registry {
	free := plugin.PluginManifest{Name: "cron", Version: "1.0.0", Tier: "free", TierPair: true}
	pro := plugin.PluginManifest{Name: "cron", Version: "1.1.0", Tier: "pro", TierPair: true, RequiresLicense: true, Bundles: []string{"claw"}}
	reg := &plugin.Registry{Plugins: []plugin.PluginManifest{pro, free}}
	if freeFirst {
		reg.Plugins = []plugin.PluginManifest{free, pro}
	}
	reg.Plugins = append(reg.Plugins,
		plugin.PluginManifest{Name: "ai", Version: "1.0.0", Tier: "pro", RequiresLicense: true, Bundles: []string{"claw"}},
		plugin.PluginManifest{Name: "dup", Version: "1.0.0", Tier: "free"},
		plugin.PluginManifest{Name: "dup", Version: "1.0.0", Tier: "pro"},
	)
	return reg
}

// TestPreflightTierPairUsesResolvedEntry: with no key and no entitlement, cron
// resolves to its free entry in either registry order, so a free bundle is not
// refused (the old map kept the LAST entry and refused free-then-pro).
func TestPreflightTierPairUsesResolvedEntry(t *testing.T) {
	for _, freeFirst := range []bool{true, false} {
		pf := licencePreflight{entitled: never}
		if err := pf.check(context.Background(), pairRegistry(freeFirst), []string{"cron"}); err != nil {
			t.Errorf("freeFirst=%v: no key, not entitled: %v, want nil (free cron installs)", freeFirst, err)
		}
	}
	// Entitled to the pro entry but no key at all: the obvious miss.
	pf := licencePreflight{entitled: always}
	if err := pf.check(context.Background(), pairRegistry(true), []string{"cron"}); !errors.Is(err, errNoLicenceKey) {
		t.Errorf("entitled pro cron without a key = %v, want errNoLicenceKey", err)
	}
}

// TestPreflightCachedEntitlement: a cached answer that omits a licensed member
// refuses it; an all-access tier, a listed plugin or no cache allows it.
func TestPreflightCachedEntitlement(t *testing.T) {
	ctx := context.Background()
	reg := pairRegistry(true)
	cases := []struct {
		name  string
		cache *cachedEntitlements
		want  bool // refused
	}{
		{"miss on a single-bundle tier", &cachedEntitlements{Tier: "chat", Plugins: []string{"chat"}}, true},
		{"listed", &cachedEntitlements{Tier: "claw", Plugins: []string{"AI"}}, false},
		{"plus has an empty list by design", &cachedEntitlements{Tier: "plus"}, false},
		{"owner", &cachedEntitlements{Tier: "Owner"}, false},
		{"unknown tier is not all-access", &cachedEntitlements{Tier: "gold"}, true},
		{"no cache", nil, false},
	}
	for _, c := range cases {
		pf := licencePreflight{entitled: never, keys: 1, cache: c.cache, cacheDir: "/x"}
		err := pf.check(ctx, reg, []string{"cron", "ai"})
		if got := err != nil; got != c.want {
			t.Errorf("%s: err = %v, want refused=%v", c.name, err, c.want)
		}
		if err != nil && !strings.Contains(err.Error(), `"ai"`) {
			t.Errorf("%s: refusal does not name the member: %v", c.name, err)
		}
	}
}

// TestPreflightNotFoundAndDuplicates: a member missing from the registry is left
// to plugin.Install; a same-slug collision that is not a tier pair is refused.
func TestPreflightNotFoundAndDuplicates(t *testing.T) {
	pf := licencePreflight{entitled: never, keys: 1}
	if err := pf.check(context.Background(), pairRegistry(true), []string{"missing"}); err != nil {
		t.Errorf("missing member: %v, want nil", err)
	}
	if err := pf.check(context.Background(), pairRegistry(true), []string{"dup"}); err == nil || !strings.Contains(err.Error(), `"dup"`) {
		t.Errorf("duplicate slug: %v, want an error naming dup", err)
	}
}

func writeEntitlements(t *testing.T, dir, tier string, plugins []string, at time.Time) {
	t.Helper()
	data, _ := json.Marshal(cachedEntitlements{Tier: tier, Plugins: plugins, CachedAt: at.UTC().Format(time.RFC3339)})
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, entitlementCacheFile), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestReadCachedEntitlements: missing, damaged or stale files are no answer.
func TestReadCachedEntitlements(t *testing.T) {
	now := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	if _, ok := readCachedEntitlements(dir, now); ok {
		t.Error("missing file read as an answer")
	}
	writeEntitlements(t, dir, "chat", []string{"chat"}, now.Add(-time.Hour))
	if c, ok := readCachedEntitlements(dir, now); !ok || c.Tier != "chat" {
		t.Errorf("fresh file = %+v %v", c, ok)
	}
	writeEntitlements(t, dir, "chat", []string{"chat"}, now.Add(-25*time.Hour))
	if _, ok := readCachedEntitlements(dir, now); ok {
		t.Error("stale file read as an answer")
	}
	if err := os.WriteFile(filepath.Join(dir, entitlementCacheFile), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := readCachedEntitlements(dir, now); ok {
		t.Error("damaged file read as an answer")
	}
}

// TestLicenceCheckerFreeBundleOnly: end to end through the seeded registry
// cache and ~/.nself/license/entitlements.json. One fake key cached for tier
// chat: a free bundle with the licensed member ai is refused; the paid-bundle
// form (Phase 1 already granted the bundle) and a second key are not.
func TestLicenceCheckerFreeBundleOnly(t *testing.T) {
	isolate(t)
	for _, k := range []string{"NSELF_PLUGIN_LICENSE_KEY_OWNER", "NSELF_LICENSE_KEY_1", "NSELF_LICENSE_KEY_2"} {
		t.Setenv(k, "")
	}
	setupOfflineRegistry(t, []byte(`{"version":"test","tier":"pro","plugins":{
    "ai":{"name":"ai","version":"1.0.0","status":"stable","tier":"pro","requires_license":true}}}`))
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "nself_pro_FAKE_TEST_KEY_NOT_REAL")
	writeEntitlements(t, plugin.LicenseCacheDir(), "chat", []string{"chat"}, time.Now())

	// The licence server is stubbed: this test must never reach the network.
	saved := validateRemote
	t.Cleanup(func() { validateRemote = saved })
	validateRemote = func(context.Context, string, string) (bool, *plugin.LicenseValidateResponse, error) {
		return false, nil, errors.New("offline in test")
	}

	ctx := context.Background()
	if err := licenceChecker(true)(ctx, []string{"ai"}); err == nil || !strings.Contains(err.Error(), "entitlements") {
		t.Errorf("free bundle, cached miss: %v, want a refusal", err)
	}
	if err := licenceChecker(false)(ctx, []string{"ai"}); err != nil {
		t.Errorf("paid bundle ignores the cached list: %v", err)
	}
	t.Setenv("NSELF_LICENSE_KEY_1", "nself_pro_FAKE_SECOND_KEY_NOT_REAL")
	if err := licenceChecker(true)(ctx, []string{"ai"}); err != nil {
		t.Errorf("two keys: the cache holds one key's answer, so it must not refuse: %v", err)
	}
}

// TestPreflightConfirmCachedMiss: a cached miss is confirmed with the licence
// server before it refuses (D-0475); a cached HIT never asks.
func TestPreflightConfirmCachedMiss(t *testing.T) {
	miss := &cachedEntitlements{Tier: "chat", Plugins: []string{"chat"}}
	hit := &cachedEntitlements{Tier: "claw", Plugins: []string{"ai"}}
	cases := []struct {
		name      string
		cache     *cachedEntitlements
		confirm   func(context.Context, string) (bool, error) // nil: no confirm func
		wantCalls int
		wantErr   []string // substrings; nil: no error
	}{
		{"server allows", miss, func(context.Context, string) (bool, error) { return true, nil }, 1, nil},
		{"server denies", miss, func(context.Context, string) (bool, error) { return false, nil }, 1,
			[]string{`"ai"`, "licence server confirmed", "nself.org/pricing"}},
		{"server unreachable", miss, func(context.Context, string) (bool, error) { return false, errors.New("dial timeout") }, 1,
			[]string{`"ai"`, entitlementCacheFile, "could not be reached", "dial timeout"}},
		{"no confirm func keeps the cached refusal", miss, nil, 0,
			[]string{`"ai"`, "cached entitlements", entitlementCacheFile}},
		{"cached hit never asks", hit, func(context.Context, string) (bool, error) { return false, nil }, 0, nil},
	}
	for _, c := range cases {
		calls := 0
		pf := licencePreflight{entitled: never, keys: 1, cache: c.cache, cacheDir: "/x"}
		if c.confirm != nil {
			pf.confirm = func(ctx context.Context, name string) (bool, error) {
				calls++
				return c.confirm(ctx, name)
			}
		}
		err := pf.check(context.Background(), pairRegistry(true), []string{"ai"})
		if calls != c.wantCalls {
			t.Errorf("%s: confirm called %d times, want %d", c.name, calls, c.wantCalls)
		}
		if c.wantErr == nil {
			if err != nil {
				t.Errorf("%s: err = %v, want nil", c.name, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s: want a refusal", c.name)
			continue
		}
		for _, sub := range c.wantErr {
			if !strings.Contains(err.Error(), sub) {
				t.Errorf("%s: %q missing %q", c.name, err, sub)
			}
		}
	}
}

// TestRemoteConfirmer: the production confirm func judges the server answer
// (valid + all-access tier or a listed plugin) and never leaks the key.
func TestRemoteConfirmer(t *testing.T) {
	const key = "nself_pro_TESTONLYFAKE"
	saved := validateRemote
	t.Cleanup(func() { validateRemote = saved })
	t.Setenv("NSELF_PING_API_URL", "http://ping.test/")
	cases := []struct {
		name  string
		valid bool
		resp  *plugin.LicenseValidateResponse
		err   error
		want  bool
	}{
		{"listed, any case", true, &plugin.LicenseValidateResponse{Tier: "claw", Plugins: []string{" AI "}}, nil, true},
		{"all-access tier", true, &plugin.LicenseValidateResponse{Tier: "plus"}, nil, true},
		{"not listed", true, &plugin.LicenseValidateResponse{Tier: "chat", Plugins: []string{"chat"}}, nil, false},
		{"invalid key", false, &plugin.LicenseValidateResponse{Tier: "plus"}, nil, false},
		{"nil response", true, nil, nil, false},
		{"transport error", false, nil, errors.New("boom"), false},
	}
	for _, c := range cases {
		var gotURL string
		validateRemote = func(_ context.Context, k, url string) (bool, *plugin.LicenseValidateResponse, error) {
			gotURL = url
			if k != key {
				t.Errorf("%s: wrong key passed", c.name)
			}
			return c.valid, c.resp, c.err
		}
		got, err := remoteConfirmer(key)(context.Background(), "ai")
		if got != c.want || (err != nil) != (c.err != nil) {
			t.Errorf("%s: got %v, %v; want %v, err=%v", c.name, got, err, c.want, c.err)
		}
		if gotURL != "http://ping.test" {
			t.Errorf("%s: ping URL = %q, want the env value without a trailing slash", c.name, gotURL)
		}
		if err != nil && strings.Contains(err.Error(), key) {
			t.Errorf("%s: error leaks the key", c.name)
		}
	}
}
