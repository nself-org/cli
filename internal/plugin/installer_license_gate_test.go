package plugin

// installer_license_gate_test.go — regression coverage for WHERE the install
// license gate runs.
//
// The defect: installLocked gated on isPaidPlugin(name), a static 59-name
// allowlist, BEFORE fetching the registry and resolving the tier. Tier is not
// a property of a name — "cron" and "notify" are each served twice, free and
// pro — so an unlicensed operator was refused before ResolvePlugin could pick
// the free entry, making the two plugins the ɳTask docs advertise as free
// impossible to install. tier_resolve_test.go covers resolution itself; these
// tests cover the gate that resolution feeds.

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestInstallGate_TierPairFreeEntryIsNotPaid pins the exact composition that
// was broken: an unentitled operator resolves a tier pair to the FREE entry,
// and that entry must not then be treated as license-gated.
func TestInstallGate_TierPairFreeEntryIsNotPaid(t *testing.T) {
	m, err := ResolvePlugin(context.Background(), fixtureRegistry(), "cron", "", neverEntitled)
	if err != nil {
		t.Fatalf("resolving cron without entitlement: %v", err)
	}
	if m.Tier != "free" {
		t.Fatalf("expected the free entry, got tier=%q", m.Tier)
	}
	if isPaidPluginManifest(m) {
		t.Error("the free half of a tier pair was classified as paid; an unlicensed operator cannot install it")
	}
	// The name-based map disagrees — that disagreement is the whole bug, so
	// assert it explicitly rather than leaving it implicit.
	if !isPaidPlugin("cron") {
		t.Skip("paidPlugins no longer lists cron; this test's premise has changed")
	}
}

// TestInstallGate_EntitledTierPairProEntryIsPaid is the other half: when
// entitlement does resolve to pro, the gate must still fire.
func TestInstallGate_EntitledTierPairProEntryIsPaid(t *testing.T) {
	m, err := ResolvePlugin(context.Background(), fixtureRegistry(), "cron", "", alwaysEntitled)
	if err != nil {
		t.Fatalf("resolving cron with entitlement: %v", err)
	}
	if !isPaidPluginManifest(m) {
		t.Errorf("the pro half of a tier pair must stay license-gated, got tier=%q", m.Tier)
	}
}

// TestInstallGate_ProOnlyPluginIsPaidRegardlessOfAllowlist covers the
// tightening the move buys: a pro registry entry is gated on its own
// metadata, including the 23 paid plugins the static allowlist omits.
func TestInstallGate_ProOnlyPluginIsPaidRegardlessOfAllowlist(t *testing.T) {
	m, err := ResolvePlugin(context.Background(), fixtureRegistry(), "search-pro-only", "", neverEntitled)
	if err != nil {
		t.Fatalf("resolving search-pro-only: %v", err)
	}
	if !isPaidPluginManifest(m) {
		t.Error("a pro-tier entry must be license-gated even though it is absent from paidPlugins")
	}
	if isPaidPlugin("search-pro-only") {
		t.Fatal("fixture premise broken: this name should NOT be in the static allowlist")
	}
}

// TestInstallGate_NotGatedBeforeRegistryFetch is a structural guard. The gate
// must not move back above the registry fetch: at that point the tier of a
// tier_pair slug is simply unknown, so any name-based decision there is a
// guess. Reading the source is the only way to assert ordering without a
// live registry.
func TestInstallGate_NotGatedBeforeRegistryFetch(t *testing.T) {
	src, err := os.ReadFile("installer_locked.go")
	if err != nil {
		t.Fatalf("reading installer_locked.go: %v", err)
	}
	body := string(src)

	fetch := strings.Index(body, "FetchRegistry(")
	if fetch < 0 {
		t.Fatal("FetchRegistry call not found; this guard needs updating")
	}
	gate := strings.Index(body, "isPaidPluginManifest(manifest)")
	if gate < 0 {
		t.Fatal("manifest-based license gate not found in installLocked")
	}
	if gate < fetch {
		t.Error("the license gate runs before the registry fetch; tier is unknown there")
	}
	if i := strings.Index(body, "isPaidPlugin(name)"); i >= 0 {
		t.Errorf("installLocked gates on the static name allowlist at offset %d; "+
			"use isPaidPluginManifest on the resolved manifest instead", i)
	}
}
