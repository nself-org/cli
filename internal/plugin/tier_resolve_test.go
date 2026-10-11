package plugin

// tier_resolve_test.go — table-driven coverage for install-time tier
// resolution (OWNER-ACTIONS.md item 15). Fixture registry: a genuine tier
// pair (cron: free + pro, both TierPair:true, pro belongs to bundle "claw"),
// a plain free-only plugin (webhooks-free-only, single entry), a plain
// pro-only plugin (search-pro-only, single entry), and a bad duplicate
// (search-collision: two entries, same slug, neither TierPair) matching the
// "distinct products sharing a slug" case OWNER-ACTIONS.md flags for search
// before the plugins-pro rename lands.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

func fixtureRegistry() *Registry {
	return &Registry{
		Plugins: []PluginManifest{
			{Name: "cron", Version: "1.0.0", Tier: "free", TierPair: true},
			{Name: "cron", Version: "1.1.2", Tier: "pro", TierPair: true, Bundles: []string{"claw"}},

			{Name: "webhooks-free-only", Version: "1.0.0", Tier: "free"},

			{Name: "search-pro-only", Version: "1.0.0", Tier: "pro", Bundles: []string{"claw"}},

			// Bad duplicate: same slug, neither side flagged tier_pair — must
			// hard-error, never silently resolve to the first entry.
			{Name: "search-collision", Version: "1.0.0", Tier: "free"},
			{Name: "search-collision", Version: "1.0.0", Tier: "pro"},
		},
	}
}

func alwaysEntitled(ctx context.Context, bundles []string) (bool, error) { return true, nil }
func neverEntitled(ctx context.Context, bundles []string) (bool, error)  { return false, nil }
func entitlementErrors(ctx context.Context, bundles []string) (bool, error) {
	return false, errors.New("ping_api unreachable")
}

func TestResolvePlugin_TierPair_EntitledDefaultsToPro(t *testing.T) {
	reg := fixtureRegistry()
	m, err := ResolvePlugin(context.Background(), reg, "cron", "", alwaysEntitled)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Tier != "pro" || m.Version != "1.1.2" {
		t.Fatalf("expected pro 1.1.2, got tier=%s version=%s", m.Tier, m.Version)
	}
}

func TestResolvePlugin_TierPair_NotEntitledDefaultsToFree(t *testing.T) {
	reg := fixtureRegistry()
	m, err := ResolvePlugin(context.Background(), reg, "cron", "", neverEntitled)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Tier != "free" || m.Version != "1.0.0" {
		t.Fatalf("expected free 1.0.0, got tier=%s version=%s", m.Tier, m.Version)
	}
}

func TestResolvePlugin_TierPair_EntitlementCheckErrorFailsClosedToFree(t *testing.T) {
	reg := fixtureRegistry()
	m, err := ResolvePlugin(context.Background(), reg, "cron", "", entitlementErrors)
	if err != nil {
		t.Fatalf("unexpected error (should fail closed to free, not error): %v", err)
	}
	if m.Tier != "free" {
		t.Fatalf("expected fail-closed free, got tier=%s", m.Tier)
	}
}

func TestResolvePlugin_TierPair_ExplicitFreeOverrideIgnoresEntitlement(t *testing.T) {
	reg := fixtureRegistry()
	// Even with an entitled license, --tier free must still return free.
	m, err := ResolvePlugin(context.Background(), reg, "cron", "free", alwaysEntitled)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Tier != "free" {
		t.Fatalf("expected free override, got tier=%s", m.Tier)
	}
}

func TestResolvePlugin_TierPair_ExplicitProOverrideStillChecksEntitlement(t *testing.T) {
	reg := fixtureRegistry()
	_, err := ResolvePlugin(context.Background(), reg, "cron", "pro", neverEntitled)
	if err == nil {
		t.Fatal("expected an error: --tier pro without entitlement must not silently succeed")
	}
	if !errors.Is(err, errs.ErrTierNotEntitled) {
		t.Fatalf("expected errs.ErrTierNotEntitled, got: %v", err)
	}
}

func TestNotEntitledBundleWording(t *testing.T) {
	one := notEntitledError("example", &PluginManifest{Bundles: []string{"Chat"}}).Error()
	if !strings.Contains(one, "requires a licence for the Chat Bundle") || strings.Contains(one, "one of these") {
		t.Fatal(one)
	}
	err := notEntitledError("example", &PluginManifest{Bundles: []string{"Chat", "AI"}})
	if !errors.Is(err, errs.ErrTierNotEntitled) {
		t.Fatal(err)
	}
	got := err.Error()
	if !strings.Contains(got, "one of these Bundles: Chat, AI") || strings.Contains(got, "Chat/AI Bundle") || strings.Contains(got, "Licensed (in") {
		t.Fatal(got)
	}
}

func TestResolvePlugin_TierPair_ExplicitProOverrideSucceedsWhenEntitled(t *testing.T) {
	reg := fixtureRegistry()
	m, err := ResolvePlugin(context.Background(), reg, "cron", "pro", alwaysEntitled)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Tier != "pro" {
		t.Fatalf("expected pro override, got tier=%s", m.Tier)
	}
}

func TestResolvePlugin_SingleFreeEntry_NoOverride(t *testing.T) {
	reg := fixtureRegistry()
	m, err := ResolvePlugin(context.Background(), reg, "webhooks-free-only", "", neverEntitled)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Tier != "free" {
		t.Fatalf("expected the plugin's only entry (free), got tier=%s", m.Tier)
	}
}

func TestResolvePlugin_SingleFreeEntry_ProOverrideRejected(t *testing.T) {
	reg := fixtureRegistry()
	_, err := ResolvePlugin(context.Background(), reg, "webhooks-free-only", "pro", alwaysEntitled)
	if err == nil {
		t.Fatal("expected an error: this slug has no pro entry")
	}
}

func TestResolvePlugin_SingleProEntry_NoOverrideDoesNotRequireEntitlement(t *testing.T) {
	// A standalone pro plugin (not a tier pair) is resolved by findPlugin's
	// original single-entry path — its own checkLicense gate (installLocked
	// Step 1) is what enforces licensing, not ResolvePlugin.
	reg := fixtureRegistry()
	m, err := ResolvePlugin(context.Background(), reg, "search-pro-only", "", neverEntitled)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Tier != "pro" {
		t.Fatalf("expected the plugin's only entry (pro), got tier=%s", m.Tier)
	}
}

func TestResolvePlugin_NonTierPairDuplicate_HardErrorsNamingBothEntries(t *testing.T) {
	reg := fixtureRegistry()
	_, err := ResolvePlugin(context.Background(), reg, "search-collision", "", alwaysEntitled)
	if err == nil {
		t.Fatal("expected errs.ErrDuplicatePluginSlug, got nil (silent first-match regression)")
	}
	if !errors.Is(err, errs.ErrDuplicatePluginSlug) {
		t.Fatalf("expected errs.ErrDuplicatePluginSlug, got: %v", err)
	}
	// Both entries must be named in the message — never resolved by picking one.
	msg := err.Error()
	if !strings.Contains(msg, "search-collision") {
		t.Fatalf("error should name the colliding slug: %v", err)
	}
}

func TestResolvePlugin_NotFound(t *testing.T) {
	reg := fixtureRegistry()
	_, err := ResolvePlugin(context.Background(), reg, "does-not-exist", "", alwaysEntitled)
	if !errors.Is(err, errs.ErrPluginNotFound) {
		t.Fatalf("expected errs.ErrPluginNotFound, got: %v", err)
	}
}

func TestResolvePlugin_InvalidTierFlagRejected(t *testing.T) {
	reg := fixtureRegistry()
	_, err := ResolvePlugin(context.Background(), reg, "cron", "enterprise", alwaysEntitled)
	if err == nil {
		t.Fatal("expected an error for an unrecognised --tier value")
	}
}

// TestSplitTierPair_DirectUnit covers splitTierPair in isolation (order
// independence and the "not a real pair" branches) since ResolvePlugin's
// table above only exercises it indirectly.
func TestSplitTierPair_DirectUnit(t *testing.T) {
	free := &PluginManifest{Name: "cron", Tier: "free", TierPair: true}
	pro := &PluginManifest{Name: "cron", Tier: "pro", TierPair: true, Bundles: []string{"claw"}}

	if f, p, ok := splitTierPair([]*PluginManifest{free, pro}); !ok || f != free || p != pro {
		t.Fatalf("free-then-pro: expected ok with (free,pro), got ok=%v f=%v p=%v", ok, f, p)
	}
	if f, p, ok := splitTierPair([]*PluginManifest{pro, free}); !ok || f != free || p != pro {
		t.Fatalf("pro-then-free: expected ok with (free,pro), got ok=%v f=%v p=%v", ok, f, p)
	}

	notPair := &PluginManifest{Name: "search-collision", Tier: "free"}
	notPair2 := &PluginManifest{Name: "search-collision", Tier: "pro"}
	if _, _, ok := splitTierPair([]*PluginManifest{notPair, notPair2}); ok {
		t.Fatal("neither entry declares TierPair — must not resolve as a pair")
	}

	twoFree := &PluginManifest{Name: "x", Tier: "free", TierPair: true}
	twoFreeB := &PluginManifest{Name: "x", Tier: "free", TierPair: true}
	if _, _, ok := splitTierPair([]*PluginManifest{twoFree, twoFreeB}); ok {
		t.Fatal("two free entries is not a valid tier pair")
	}
}

// resolveTier resolves name with the installed tier and --tier given.
func resolveTier(t *testing.T, name, override, installed string, ent EntitlementFunc) (*PluginManifest, string, error) {
	t.Helper()
	return ResolvePluginTier(context.Background(), fixtureRegistry(), name, override, installed, ent)
}

// TestTierResolveSticky: a licence bought after a free install must not move
// the plugin to Licensed on update (v1.5); v1.4 keeps resolving by entitlement.
func TestTierResolveSticky(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		v15 := os.Getenv("NSELF_V15") == "1"
		// Installed free, licence now entitles pro.
		m, reason, err := resolveTier(t, "cron", "", "free", alwaysEntitled)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		wantTier, wantReason := "pro", TierReasonEntitlement
		if v15 {
			wantTier, wantReason = "free", TierReasonInstalled
		}
		if m.Tier != wantTier || reason != wantReason {
			t.Fatalf("installed free + entitled: got %s/%s, want %s/%s", m.Tier, reason, wantTier, wantReason)
		}
		// Installed pro while still entitled stays pro (reason installed in v1.5).
		m, reason, err = resolveTier(t, "cron", "", "pro", alwaysEntitled)
		if err != nil || m.Tier != "pro" {
			t.Fatalf("installed pro + entitled: got %v/%v, want pro", m, err)
		}
		if v15 && reason != TierReasonInstalled {
			t.Fatalf("reason = %q, want %q", reason, TierReasonInstalled)
		}
		// Not installed at all: entitlement decides, in both modes.
		m, reason, err = resolveTier(t, "cron", "", "", alwaysEntitled)
		if err != nil || m.Tier != "pro" || reason != TierReasonEntitlement {
			t.Fatalf("fresh install entitled: got %v/%s/%v", m, reason, err)
		}
		m, reason, err = resolveTier(t, "cron", "", "", neverEntitled)
		if err != nil || m.Tier != "free" || reason != TierReasonDefaultFree {
			t.Fatalf("fresh install not entitled: got %v/%s/%v", m, reason, err)
		}
	})
}

// TestTierResolveOverride: --tier always wins over the installed tier and
// still runs the entitlement check for the Licensed side.
func TestTierResolveOverride(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		m, reason, err := resolveTier(t, "cron", "licensed", "free", alwaysEntitled)
		if err != nil || m.Tier != "pro" || reason != TierReasonOverride {
			t.Fatalf("--tier licensed over installed free: got %v/%s/%v", m, reason, err)
		}
		m, reason, err = resolveTier(t, "cron", "free", "pro", alwaysEntitled)
		if err != nil || m.Tier != "free" || reason != TierReasonOverride {
			t.Fatalf("--tier free over installed pro: got %v/%s/%v", m, reason, err)
		}
		if _, _, err = resolveTier(t, "cron", "licensed", "free", neverEntitled); !errors.Is(err, errs.ErrTierNotEntitled) {
			t.Fatalf("--tier licensed without a licence must stay refused, got %v", err)
		}
		if _, _, err = resolveTier(t, "cron", "enterprise", "free", alwaysEntitled); err == nil {
			t.Fatal("invalid --tier must be rejected")
		}
	})
}

// TestTierChangeNeedsFlagE131: when the installed tier cannot be kept, v1.5
// stops with E131 instead of switching; --tier makes the switch on purpose.
func TestTierChangeNeedsFlagE131(t *testing.T) {
	wantE131 := func(t *testing.T, err error) {
		t.Helper()
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E131" {
			t.Fatalf("want E131, got %v", err)
		}
	}
	t.Run("v1.5", func(t *testing.T) {
		compattest.Set(t, true)
		// Licence lapsed: installed pro would fall to free.
		_, _, err := resolveTier(t, "cron", "", "pro", neverEntitled)
		wantE131(t, err)
		if !errors.Is(err, errs.ErrTierNotEntitled) {
			t.Fatalf("E131 should wrap ErrTierNotEntitled, got %v", err)
		}
		if m, _, err := resolveTier(t, "cron", "free", "pro", neverEntitled); err != nil || m.Tier != "free" {
			t.Fatalf("--tier free must make the change, got %v/%v", m, err)
		}
		// Registry now serves only the other tier.
		_, _, err = resolveTier(t, "search-pro-only", "", "free", alwaysEntitled)
		wantE131(t, err)
		_, _, err = resolveTier(t, "webhooks-free-only", "", "pro", alwaysEntitled)
		wantE131(t, err)
		if m, _, err := resolveTier(t, "search-pro-only", "licensed", "free", alwaysEntitled); err != nil || m.Tier != "pro" {
			t.Fatalf("--tier licensed must make the change, got %v/%v", m, err)
		}
		// An entitlement lookup that fails is not a tier change: no downgrade.
		_, _, err = resolveTier(t, "cron", "", "pro", entitlementErrors)
		if err == nil || strings.Contains(err.Error(), "E131") {
			t.Fatalf("lookup failure must surface as itself, got %v", err)
		}
		if m, _, err := resolveTier(t, "webhooks-free-only", "", "free", alwaysEntitled); err != nil || m.Tier != "free" {
			t.Fatalf("same tier must pass, got %v/%v", m, err)
		}
	})
	t.Run("v1.4", func(t *testing.T) {
		compattest.Set(t, false)
		m, _, err := resolveTier(t, "cron", "", "pro", neverEntitled)
		if err != nil || m.Tier != "free" {
			t.Fatalf("v1.4 keeps today's behaviour (free), got %v/%v", m, err)
		}
		if m, _, err := resolveTier(t, "search-pro-only", "", "free", alwaysEntitled); err != nil || m.Tier != "pro" {
			t.Fatalf("v1.4 single entry returned as-is, got %v/%v", m, err)
		}
	})
}

// TestInstalledTier: what `plugin info --json` reports for an installed copy.
func TestInstalledTier(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name, "plugin.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("notify", `{"name":"notify","version":"1.0.0","description":"d","tier":"free"}`)
	write("claw", `{"name":"claw","version":"1.0.0","description":"d","tier":"pro"}`)
	if tier, reason, ok := InstalledTier(dir, "notify"); !ok || tier != "free" || reason != TierReasonInstalled {
		t.Fatalf("notify: got %q/%q/%v", tier, reason, ok)
	}
	if tier, _, ok := InstalledTier(dir, "claw"); !ok || tier != "licensed" {
		t.Fatalf("claw: got %q/%v, want licensed", tier, ok)
	}
	if _, _, ok := InstalledTier(dir, "absent"); ok {
		t.Fatal("a plugin that is not installed has no installed tier")
	}
}

// TestUpdateKeepsInstalledTier drives Update itself: a Licensed install whose
// licence is gone stops with E131 (v1.5) and the previous copy is restored;
// no process-environment variable carries the tier.
func TestUpdateKeepsInstalledTier(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"plugins":[` +
			`{"name":"cron","version":"1.0.0","description":"d","category":"c","license":"MIT","tier":"free","tier_pair":true},` +
			`{"name":"cron","version":"1.1.2","description":"d","category":"c","license":"x","tier":"pro","tier_pair":true,"bundles":["claw"]}]}`))
	}))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("HOME", home)        // registry cache lands here
	t.Setenv("USERPROFILE", home) // os.UserHomeDir on Windows
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	t.Setenv("NSELF_PLUGIN_INSTALL_TIER", "")

	pluginDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(pluginDir, "cron"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"cron","version":"1.0.0","description":"d","category":"c","license":"x","tier":"pro"}`
	if err := os.WriteFile(filepath.Join(pluginDir, "cron", "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	compattest.Set(t, true)
	err := Update(context.Background(), &config.Config{}, "cron", pluginDir)
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E131" {
		t.Fatalf("want E131 from Update, got %v", err)
	}
	if b, rerr := os.ReadFile(filepath.Join(pluginDir, "cron", "plugin.json")); rerr != nil || string(b) != manifest {
		t.Fatalf("the previous copy must be restored untouched: %v", rerr)
	}
	// The failed update must leave no .prev backup behind and the tier must
	// have come from the on-disk copy alone (the env var is not set).
	if _, serr := os.Stat(filepath.Join(pluginDir, "cron.prev")); !os.IsNotExist(serr) {
		t.Fatalf("a failed update must put the backup back: %v", serr)
	}
	if v := os.Getenv("NSELF_PLUGIN_INSTALL_TIER"); v != "" {
		t.Fatalf("Update must not set the tier override itself: %q", v)
	}
}

// TestUpdateWithTierSwitchesOnPurpose: the same lapsed-licence update that
// stops with E131 gets past tier resolution once the --tier override is set.
func TestUpdateWithTierSwitchesOnPurpose(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"plugins":[` +
			`{"name":"cron","version":"1.0.0","description":"d","category":"c","license":"MIT","tier":"free","tier_pair":true},` +
			`{"name":"cron","version":"1.1.2","description":"d","category":"c","license":"x","tier":"pro","tier_pair":true,"bundles":["claw"]}]}`))
	}))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir on Windows
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	t.Setenv("NSELF_PLUGIN_INSTALL_TIER", "")
	pluginDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(pluginDir, "cron"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"cron","version":"1.0.0","description":"d","category":"c","license":"x","tier":"pro"}`
	if err := os.WriteFile(filepath.Join(pluginDir, "cron", "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	compattest.Set(t, true)
	t.Setenv("NSELF_PLUGIN_INSTALL_TIER", "free") // what the cmd layer sets for --tier free
	err := Update(context.Background(), &config.Config{}, "cron", pluginDir)
	var ce *errs.CLIError
	if errors.As(err, &ce) && ce.Code == "E131" {
		t.Fatalf("--tier free must not stop with E131: %v", err)
	}
	if err == nil {
		return
	}
	if !strings.Contains(err.Error(), "installing updated plugin") {
		t.Fatalf("expected the update to reach the install step, got %v", err)
	}
}

// installedPair writes a registry server serving the cron tier pair and a
// plugin dir holding cron installed with the given wire tier.
func installedPair(t *testing.T, installedWire string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"plugins":[` +
			`{"name":"cron","version":"1.0.0","description":"d","category":"c","license":"MIT","tier":"free","tier_pair":true},` +
			`{"name":"cron","version":"1.1.2","description":"d","category":"c","license":"x","tier":"pro","tier_pair":true,"bundles":["claw"]}]}`))
	}))
	t.Cleanup(srv.Close)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir on Windows
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	t.Setenv("NSELF_PLUGIN_INSTALL_TIER", "")
	pluginDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(pluginDir, "cron"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"cron","version":"1.0.0","description":"d","category":"c","license":"x","tier":"` + installedWire + `"}`
	if err := os.WriteFile(filepath.Join(pluginDir, "cron", "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return pluginDir
}

// TestInstallKeepsInstalledTier (CR S1): a plain `plugin install cron` over an
// installed Licensed copy, with the licence gone, stops with E131 in v1.5
// instead of silently resolving to the free entry; v1.4 keeps the old
// entitlement-only resolution.
func TestInstallKeepsInstalledTier(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		pluginDir := installedPair(t, "pro")
		err := Install(context.Background(), &config.Config{}, "cron", pluginDir)
		var ce *errs.CLIError
		isE131 := errors.As(err, &ce) && ce.Code == "E131"
		if compat.V15() && !isE131 {
			t.Fatalf("v1.5: reinstall over an installed licensed copy must stop with E131, got %v", err)
		}
		if !compat.V15() && isE131 {
			t.Fatalf("v1.4: no E131 (base behaviour), got %v", err)
		}
	})
}

// TestInstalledTierForInstall: the live copy wins, the .prev backup Update
// parks it in stands in while the live copy is gone, a fresh install has none.
func TestInstalledTierForInstall(t *testing.T) {
	dir := t.TempDir()
	write := func(sub, tier string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, sub, "plugin.json"), []byte(`{"tier":"`+tier+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := installedTierForInstall(dir, "cron"); got != "" {
		t.Fatalf("fresh install: got %q, want empty", got)
	}
	write("cron.prev", "pro")
	if got := installedTierForInstall(dir, "cron"); got != "pro" {
		t.Fatalf(".prev backup: got %q, want pro", got)
	}
	write("cron", "free")
	if got := installedTierForInstall(dir, "cron"); got != "free" {
		t.Fatalf("live copy must win over .prev: got %q, want free", got)
	}
}

// TestRemoveClearsPrevBackup: Remove also deletes the "<name>.prev" backup an
// interrupted Update left, so a later fresh install does not inherit its tier.
func TestRemoveClearsPrevBackup(t *testing.T) {
	dir := t.TempDir()
	for sub, tier := range map[string]string{"cron": "free", "cron.prev": "pro"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, sub, "plugin.json"), []byte(`{"tier":"`+tier+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := Remove(context.Background(), &config.Config{}, "cron", dir, true, true); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cron.prev")); !os.IsNotExist(err) {
		t.Fatalf("cron.prev must be gone after Remove, stat err %v", err)
	}
	if got := installedTierForInstall(dir, "cron"); got != "" {
		t.Fatalf("fresh install after Remove inherited tier %q, want empty", got)
	}
}

// TestResolveMaxTierAndOnlyTier: a "max" entry is a licensed entry. --tier
// licensed matches it (M1), a single licensed entry with nothing installed and
// no override reports only-tier because no entitlement was looked up (S3), and
// an installed max copy sticks.
func TestResolveMaxTierAndOnlyTier(t *testing.T) {
	reg := &Registry{Plugins: []PluginManifest{{Name: "claw", Version: "1.0.0", Tier: "max", Bundles: []string{"claw"}}}}
	compattest.Both(t, func(t *testing.T) {
		m, reason, err := ResolvePluginTier(context.Background(), reg, "claw", "licensed", "", alwaysEntitled)
		if err != nil || m == nil || reason != TierReasonOverride {
			t.Fatalf("--tier licensed on a max entry: %v %v %v", m, reason, err)
		}
		if _, _, err := ResolvePluginTier(context.Background(), reg, "claw", "free", "", alwaysEntitled); err == nil {
			t.Fatal("--tier free on a max-only plugin must be refused")
		}
		entitlementCalls := 0
		counting := func(context.Context, []string) (bool, error) { entitlementCalls++; return false, nil }
		if _, reason, err = ResolvePluginTier(context.Background(), reg, "claw", "", "", counting); err != nil || reason != TierReasonOnlyTier || entitlementCalls != 0 {
			t.Fatalf("single licensed entry: reason %q err %v calls %d, want only-tier with no lookup", reason, err, entitlementCalls)
		}
		if got := LicenseTier("max"); got != "licensed" {
			t.Fatalf("LicenseTier(max) = %q, want licensed", got)
		}
		if got := LicenseTier("free"); got != "free" {
			t.Fatalf("LicenseTier(free) = %q", got)
		}
		if _, reason, err = ResolvePluginTier(context.Background(), reg, "claw", "", "max", nil); err != nil {
			t.Fatalf("installed max copy: %v", err)
		} else if compat.V15() && reason != TierReasonInstalled {
			t.Fatalf("v1.5 installed max copy: reason %q, want installed", reason)
		}
	})
}

// TestDuplicateSlugErrorUsesLicenceWord: the collision report names the
// licence word (licensed), not the raw registry tier (max).
func TestDuplicateSlugErrorUsesLicenceWord(t *testing.T) {
	reg := &Registry{Plugins: []PluginManifest{{Name: "dup", Version: "1", Tier: "max"}, {Name: "dup", Version: "2", Tier: "free"}}}
	_, err := ResolvePlugin(context.Background(), reg, "dup", "", neverEntitled)
	if err == nil || !strings.Contains(err.Error(), "license=licensed") || strings.Contains(err.Error(), "license=max") {
		t.Fatalf("want license=licensed in %v", err)
	}
}
