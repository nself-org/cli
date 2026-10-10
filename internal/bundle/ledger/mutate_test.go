package ledger

// Mutation rules (P7-PLUG-18): installed_by / explicit refcount semantics.

import (
	"reflect"
	"testing"
	"time"
)

func inst(name string) InstalledPlugin {
	return InstalledPlugin{Name: name, Version: "1.0.0", Tier: TierLicensed}
}

// overlapLedger: bundles A and B overlap on "shared"; "own" is only A's;
// "solo" is explicit and also in A.
func overlapLedger(t *testing.T) Ledger {
	t.Helper()
	l := New()
	at := fixedNow()
	l.AddBundle("a", []InstalledPlugin{inst("shared"), inst("own"), inst("solo")}, at)
	l.AddBundle("b", []InstalledPlugin{inst("shared"), inst("bonly")}, at)
	l.MarkExplicit(inst("solo"))
	if _, err := l.Marshal(); err != nil {
		t.Fatalf("setup ledger invalid: %v", err)
	}
	return l
}

func TestLedgerRemoveBundleRefcount(t *testing.T) {
	l := overlapLedger(t)
	got, err := l.RemoveBundle("a")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"own"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("removable after removing a = %v, want %v (shared is B's, solo is explicit)", got, want)
	}
	if _, ok := l.Plugins["own"]; ok {
		t.Error("own should be gone from the ledger")
	}
	if p := l.Plugins["shared"]; !reflect.DeepEqual(p.InstalledBy, []string{"b"}) {
		t.Errorf("shared installed_by = %v, want [b]", p.InstalledBy)
	}
	if p := l.Plugins["solo"]; !p.Explicit || len(p.InstalledBy) != 0 {
		t.Errorf("solo = %+v, want explicit with no bundles", p)
	}
	if _, ok := l.Bundles["a"]; ok {
		t.Error("bundle a still recorded")
	}
	if _, err := l.Marshal(); err != nil {
		t.Errorf("ledger invalid after remove: %v", err)
	}

	got, err = l.RemoveBundle("b")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"bonly", "shared"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("removable after removing b = %v, want %v", got, want)
	}
}

func TestLedgerRemoveUnknownBundle(t *testing.T) {
	l := overlapLedger(t)
	before, _ := l.Marshal()
	if _, err := l.RemoveBundle("ghost"); err == nil {
		t.Fatal("removing an unknown bundle succeeded")
	}
	after, _ := l.Marshal()
	if string(before) != string(after) {
		t.Fatal("a failed RemoveBundle changed the ledger")
	}
}

func TestLedgerAddBundleKeepsExplicit(t *testing.T) {
	l := New()
	l.MarkExplicit(inst("x"))
	l.AddBundle("a", []InstalledPlugin{inst("x")}, time.Now())
	l.AddBundle("a", []InstalledPlugin{inst("x")}, time.Now()) // idempotent
	p := l.Plugins["x"]
	if !p.Explicit || !reflect.DeepEqual(p.InstalledBy, []string{"a"}) {
		t.Fatalf("x = %+v, want explicit and installed_by [a] once", p)
	}
	// Explicit plugin survives its only bundle being removed.
	removable, err := l.RemoveBundle("a")
	if err != nil || len(removable) != 0 {
		t.Fatalf("RemoveBundle = %v, %v; want nothing removable", removable, err)
	}
	if _, ok := l.Plugins["x"]; !ok {
		t.Fatal("explicit plugin dropped with its bundle")
	}
}

// TestLedgerAddBundleKeepsFacts: callers pass members by name only (a bundle's
// plugin list is []string); recorded facts must survive, and the first
// installed_at must survive a re-add.
func TestLedgerAddBundleKeepsFacts(t *testing.T) {
	l := New()
	full := InstalledPlugin{Name: "bots", Version: "1.2.0", Tier: TierLicensed, Checksum: "sha256:" + sum1}
	l.MarkExplicit(full)
	first := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l.AddBundle("chat", []InstalledPlugin{{Name: "bots"}}, first)
	p := l.Plugins["bots"]
	if p.Tier != TierLicensed || p.Version != "1.2.0" || p.Checksum != sum1 || !p.Explicit {
		t.Fatalf("name-only AddBundle erased facts: %+v", p)
	}
	l.MarkExplicit(InstalledPlugin{Name: "bots", Tier: "pro"}) // not free|licensed: ignored
	if l.Plugins["bots"].Tier != TierLicensed || l.Plugins["bots"].Version != "1.2.0" {
		t.Fatalf("MarkExplicit with empty facts erased them: %+v", l.Plugins["bots"])
	}
	l.AddBundle(" Chat ", []InstalledPlugin{{Name: "bots", Version: "1.3.0"}}, first.Add(48*time.Hour))
	if got := l.Bundles["chat"].InstalledAt; got != "2026-01-01T00:00:00Z" {
		t.Errorf("re-add moved installed_at to %s", got)
	}
	if l.Plugins["bots"].Version != "1.3.0" || l.Plugins["bots"].Checksum != sum1 {
		t.Errorf("a carried version must update, others stay: %+v", l.Plugins["bots"])
	}
	l.AddBundle("chat", []InstalledPlugin{{Name: "fresh", Tier: "weird"}}, first)
	if f := l.Plugins["fresh"]; f.Tier != TierFree || f.Explicit {
		t.Errorf("new name-only record = %+v, want free and not explicit", f)
	}
	if _, err := l.Marshal(); err != nil {
		t.Errorf("ledger invalid: %v", err)
	}
}
