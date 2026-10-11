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
