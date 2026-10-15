package plugin

// download_tier_test.go — Regression coverage for the paid-plugin download
// routing fix (qa/bugs/plugin-distribution-broken.md).
//
// Purpose: prove that tier routing comes from the registry manifest and not
//          from the hand-maintained paidPlugins name map. The shipped bug was
//          that 68 of the 127 registered paid plugins are absent from that map,
//          so every one of them was routed down the FREE download path
//          (plugins.nself.org tarball + GitHub Releases fallback) and 404'd,
//          because they were never published as free releases.
// Inputs:  plugin names, versions, repository URLs, manifest tier fields.
// Outputs: assertions on the URL chosen and on the manifest tier predicate.
// Constraints: pure string construction — no network, no filesystem.

import (
	"strings"
	"testing"
)

// TestIsPaidPluginManifest_UsesRegistryFieldsNotNameMap is the negative test the
// bug closure requires: a plugin that the registry marks paid but that is
// missing from paidPlugins must still be recognized as paid. "storage" and
// "nself-uptime-monitor" are real examples of that gap.
func TestIsPaidPluginManifest_UsesRegistryFieldsNotNameMap(t *testing.T) {
	cases := []struct {
		name     string
		manifest *PluginManifest
		want     bool
	}{
		{"tier pro, absent from name map", &PluginManifest{Name: "storage", Tier: "pro"}, true},
		{"tier max, absent from name map", &PluginManifest{Name: "nself-uptime-monitor", Tier: "max"}, true},
		{"requires_license, no tier", &PluginManifest{Name: "storage", RequiresLicense: true}, true},
		{"free plugin", &PluginManifest{Name: "backup", Tier: "free"}, false},
		{"no tier metadata at all", &PluginManifest{Name: "backup"}, false},
		{"nil manifest", nil, false},
	}

	for _, c := range cases {
		if got := isPaidPluginManifest(c.manifest); got != c.want {
			t.Errorf("%s: isPaidPluginManifest = %v, want %v", c.name, got, c.want)
		}
	}

	// The gap itself: these names are paid in the registry and absent from the
	// static map. If someone "fixes" the drift by adding them to paidPlugins,
	// this assertion fails and points them back at the manifest predicate.
	for _, name := range []string{"storage", "nself-uptime-monitor"} {
		if isPaidPlugin(name) {
			t.Errorf("%q is in paidPlugins; the map was hand-extended instead of "+
				"relying on isPaidPluginManifest — see license.go doc comment", name)
		}
	}
}

// TestBuildDownloadURLForTier_RoutesRegistryPaidPluginToPingAPI verifies the
// consequence of the predicate: a registry-paid plugin gets the licensed
// ping.nself.org endpoint even though the name map does not know it.
func TestBuildDownloadURLForTier_RoutesRegistryPaidPluginToPingAPI(t *testing.T) {
	t.Setenv("NSELF_PING_API_URL", "https://ping.example.test")

	paidURL := buildDownloadURLForTier("storage", "1.2.0", "", true)
	if want := "https://ping.example.test/plugins/storage/download?tier=licensed"; paidURL != want {
		t.Errorf("paid URL = %q, want %q", paidURL, want)
	}

	// Same plugin routed by the legacy name-only path lands on the free worker.
	// That is precisely the shipped 404, kept here as the contrast case.
	legacyURL := buildDownloadURL("storage", "1.2.0", "")
	if !strings.Contains(legacyURL, "plugins.nself.org") {
		t.Fatalf("expected the name-map path to still hit the free worker, got %q", legacyURL)
	}
	if legacyURL == paidURL {
		t.Error("name-map path and manifest path agree; the regression this test " +
			"guards can no longer be distinguished")
	}
}

// TestBuildFallbackDownloadURL_NoVPrefixOnAssetName pins the asset filename
// format that nself-org/plugins release automation actually publishes. The tag
// carries a "v"; the asset filename does not. Emitting "name-v1.1.9.tar.gz"
// 404s against every real release.
func TestBuildFallbackDownloadURL_NoVPrefixOnAssetName(t *testing.T) {
	got := buildFallbackDownloadURL("backup", "1.1.9", "")
	want := "https://github.com/nself-org/plugins/releases/download/v1.1.9/backup-1.1.9.tar.gz"
	if got != want {
		t.Errorf("default repo fallback = %q, want %q", got, want)
	}

	gotRepo := buildFallbackDownloadURL("backup", "1.1.9", "https://github.com/acme/plugins.git")
	wantRepo := "https://github.com/acme/plugins/releases/download/v1.1.9/backup-1.1.9.tar.gz"
	if gotRepo != wantRepo {
		t.Errorf("explicit repo fallback = %q, want %q", gotRepo, wantRepo)
	}

	if strings.Contains(got, "backup-v1.1.9") {
		t.Error("asset filename regained the 'v' prefix; every release asset URL 404s")
	}
}
