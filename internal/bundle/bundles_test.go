package bundle

import (
	"testing"
	"time"

	"github.com/nself-org/cli/internal/testwatch"
)

// fixtureBundlesJSON is a representative bundles.json payload (ADR-P6-03
// shape) used to seed the resolver in tests without a network round trip.
// Mirrors real bundles.json: short canonical slugs, "sentry" includes
// nself-stripe (Ruling 2: IsInstallable never special-cases any plugin).
const fixtureBundlesJSON = `{
  "schema_version": "2.0.0",
  "bundles": {
    "task":   {"display": "Task Bundle",   "tier": "free", "price_monthly": 0,    "price_yearly": 0,     "plugins": ["notifications","jobs"]},
    "chat":   {"display": "Chat Bundle",   "tier": "paid", "price_monthly": 0.99, "price_yearly": 9.99,  "plugins": ["bots","livekit"]},
    "claw":   {"display": "ɳClaw",         "tier": "paid", "price_monthly": 0.99, "price_yearly": 9.99,  "plugins": ["ai","claw","claw-web"]},
    "family": {"display": "ɳFamily",       "tier": "paid", "price_monthly": 0.99, "price_yearly": 9.99,  "plugins": ["social","photos"]},
    "sentry": {"display": "ɳSentry",       "tier": "paid", "price_monthly": 0.99, "price_yearly": 9.99,  "plugins": ["nself-uptime-monitor","nself-status-page","nself-incident-mgmt","nself-alert-router","nself-slo-tracker","nself-synthetic-monitor","nself-rum","nself-errors","nself-cron-monitor","nself-oncall","nself-crash","nself-anomaly","nself-audit","nself-stripe"]},
    "tv":     {"display": "Media Bundle",  "tier": "paid", "price_monthly": 0.99, "price_yearly": 9.99,  "plugins": ["streaming","epg"]},
    "clawde": {"display": "ClawDE",        "tier": "paid", "price_monthly": 0.99, "price_yearly": 9.99,  "plugins": ["auth","cms","realtime"]}
  }
}`

func TestMain(m *testing.M) {
	if err := LoadBytes([]byte(fixtureBundlesJSON)); err != nil {
		panic("seeding bundle fixture: " + err.Error())
	}
	testwatch.Main(m, 90*time.Second) // a never-released operation lock fails in 90 s, not 10 min
}

func TestGet_KnownBundles(t *testing.T) {
	cases := []struct {
		slug        string
		wantOK      bool
		wantSlug    string
		installable bool
	}{
		{"claw", true, "claw", true},
		{"CLAW", true, "claw", true},
		{"  chat  ", true, "chat", true},
		{"nself-plus", true, "nself-plus", false}, // meta, not installable
		{"task", true, "task", true},              // free, installable as a unit since P7-PLUG-19
		{"nclaw", true, "claw", true},             // legacy n-prefixed alias
		{"NSENTRY ", true, "sentry", true},        // alias case-insensitive + trimmed
		{"ntv", true, "tv", true},
		{"bogus", false, "", false},
	}
	for _, tc := range cases {
		b, ok := Get(tc.slug)
		if ok != tc.wantOK {
			t.Errorf("Get(%q) ok = %v; want %v", tc.slug, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if b.Slug != tc.wantSlug {
			t.Errorf("Get(%q).Slug = %q; want %q", tc.slug, b.Slug, tc.wantSlug)
		}
		if b.IsInstallable() != tc.installable {
			t.Errorf("Get(%q).IsInstallable() = %v; want %v", tc.slug, b.IsInstallable(), tc.installable)
		}
	}
}

// TestGet_AliasAndCanonicalMatch verifies both the legacy n-prefixed form and
// the bundles.json-canonical short form resolve to the identical Bundle
// (backward-compat requirement, ticket acceptance criterion #2).
func TestGet_AliasAndCanonicalMatch(t *testing.T) {
	pairs := map[string]string{
		"nclaw":   "claw",
		"nchat":   "chat",
		"nfamily": "family",
		"ntv":     "tv",
		"nsentry": "sentry",
		"ntask":   "task",
	}
	for legacy, canonical := range pairs {
		legacyB, ok1 := Get(legacy)
		canonicalB, ok2 := Get(canonical)
		if !ok1 || !ok2 {
			t.Errorf("alias pair (%q,%q): ok1=%v ok2=%v", legacy, canonical, ok1, ok2)
			continue
		}
		if legacyB.Slug != canonicalB.Slug || legacyB.Name != canonicalB.Name {
			t.Errorf("Get(%q) = %+v; Get(%q) = %+v — must match", legacy, legacyB, canonical, canonicalB)
		}
	}
}

// TestIsInstallable_SentryIncludesStripe proves Ruling 2: IsInstallable's
// plugin set for sentry is exactly bundles.json's array, including
// nself-stripe, with no cli-side subsetting.
func TestIsInstallable_SentryIncludesStripe(t *testing.T) {
	b, ok := Get("sentry")
	if !ok {
		t.Fatal("Get(sentry) failed")
	}
	found := false
	for _, p := range b.Plugins {
		if p == "nself-stripe" {
			found = true
		}
	}
	if !found {
		t.Errorf("sentry bundle plugins %v missing nself-stripe (Ruling 2 violation)", b.Plugins)
	}
	if !b.IsInstallable() {
		t.Error("sentry bundle should be installable")
	}
}

func TestNames_SortedAndComplete(t *testing.T) {
	names := Names()
	if len(names) < 6 {
		t.Errorf("Names() returned %d entries; want at least 6", len(names))
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("Names() not sorted at %d: %q > %q", i, names[i-1], names[i])
		}
	}
	required := []string{"claw", "chat", "family", "tv", "clawde", "sentry", "task"}
	have := make(map[string]bool, len(names))
	for _, n := range names {
		have[n] = true
	}
	for _, r := range required {
		if !have[r] {
			t.Errorf("Names() missing required bundle %q", r)
		}
	}
	// Legacy aliases must never leak into Names().
	for _, alias := range []string{"nclaw", "nchat", "nfamily", "ntv", "nsentry", "ntask"} {
		if have[alias] {
			t.Errorf("Names() must not include alias %q", alias)
		}
	}
}

func TestAll_DisplayOrder(t *testing.T) {
	all := All()
	if len(all) != len(DisplayOrder) {
		t.Fatalf("All() returned %d; want %d (DisplayOrder length)", len(all), len(DisplayOrder))
	}
	for i, b := range all {
		if b.Slug != DisplayOrder[i] {
			t.Errorf("All()[%d].Slug = %q; want %q", i, b.Slug, DisplayOrder[i])
		}
	}
}

// TestSelfPlus_UnionIsComputed proves the ɳSelf+ meta-bundle's plugin list is
// the deduped union of every paid bundle's plugins, never a hand-listed set.
func TestSelfPlus_UnionIsComputed(t *testing.T) {
	plus, ok := Get("nself-plus")
	if !ok {
		t.Fatal("Get(nself-plus) failed")
	}
	plusSet := make(map[string]bool, len(plus.Plugins))
	for _, p := range plus.Plugins {
		plusSet[p] = true
	}
	for _, slug := range []string{"chat", "claw", "family", "sentry", "tv", "clawde"} {
		b, _ := Get(slug)
		for _, p := range b.Plugins {
			if !plusSet[p] {
				t.Errorf("nself-plus union missing %q from paid bundle %q", p, slug)
			}
		}
	}
	// task is free — its plugins must NOT be pulled into the paid union.
	task, _ := Get("task")
	for _, p := range task.Plugins {
		if plusSet[p] {
			t.Errorf("nself-plus union unexpectedly includes free-tier plugin %q from task", p)
		}
	}
}

func TestUnknownBundleError_ContainsHints(t *testing.T) {
	err := UnknownBundleError("bogus")
	if err == nil {
		t.Fatal("UnknownBundleError returned nil")
	}
	msg := err.Error()
	for _, want := range []string{"bogus", "claw", "sentry"} {
		if !contains(msg, want) {
			t.Errorf("error message missing %q: %s", want, msg)
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// TestBundleIsFree: only a bundle priced FREE skips the licence calls.
func TestBundleIsFree(t *testing.T) {
	task, _ := Get("task")
	claw, _ := Get("claw")
	if !task.IsFree() || claw.IsFree() {
		t.Fatalf("IsFree: task=%v claw=%v, want true/false (prices %q / %q)", task.IsFree(), claw.IsFree(), task.Price, claw.Price)
	}
	if !task.IsInstallable() {
		t.Error("task must be installable as a unit")
	}
	if p, _ := Get("nself-plus"); p.IsInstallable() {
		t.Error("nself-plus must stay non-installable")
	}
}
