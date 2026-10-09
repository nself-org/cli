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
	"strings"
	"testing"

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
