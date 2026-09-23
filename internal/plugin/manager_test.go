package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCollectRoutesIncludesBaseServices is the S01.T03 regression test.
// collectInstalledPluginRoutes must seed the returned route set with base
// service routes (Hasura /api, Auth /auth, Storage /storage) so that a plugin
// claiming one of those paths is detected as a conflict on a clean init — not
// silently permitted (the previous false-positive-free behaviour).
func TestCollectRoutesIncludesBaseServices(t *testing.T) {
	// Use an empty plugin directory — simulates a clean `nself init` with no
	// plugins installed yet.
	pluginDir := t.TempDir()

	routes := collectInstalledPluginRoutes(pluginDir, "")

	if len(routes) == 0 {
		t.Fatal("S01.T03 regression: collectInstalledPluginRoutes returned empty slice on clean init — base service routes must be seeded")
	}

	// Build a map of (serverName+location) -> pluginName for easy lookup.
	routeMap := make(map[string]string, len(routes))
	for _, r := range routes {
		routeMap[r.ServerName+r.Location] = r.PluginName
	}

	baseExpected := []struct {
		serverName string
		location   string
		owner      string
	}{
		{"api", "/", "hasura"},
		{"auth", "/", "auth"},
		{"storage", "/", "storage"},
	}

	for _, want := range baseExpected {
		key := want.serverName + want.location
		owner, found := routeMap[key]
		if !found {
			t.Errorf("S01.T03 regression: base service route %q%q not seeded in route set", want.serverName, want.location)
			continue
		}
		if owner != want.owner {
			t.Errorf("base service route %q%q has PluginName=%q, want %q", want.serverName, want.location, owner, want.owner)
		}
	}
}

// TestCollectRoutesIncludesBaseServices_PluginRoutesAppended verifies that
// installed plugin routes are appended after the base service seed so the
// combined slice contains both.
func TestCollectRoutesIncludesBaseServices_PluginRoutesAppended(t *testing.T) {
	pluginDir := t.TempDir()

	// Write a minimal manifest (no api_endpoints — the base routes alone suffice
	// for this sub-test, which just checks count).
	writeManifestFile(t, pluginDir, "my-plugin", "1.0.0", nil, false)

	routes := collectInstalledPluginRoutes(pluginDir, "")

	// Should have at least the 3 base routes.
	if len(routes) < 3 {
		t.Errorf("expected at least 3 routes (base services), got %d", len(routes))
	}

	// Confirm base routes are present.
	var foundBase int
	for _, r := range routes {
		if r.ServerName == "api" || r.ServerName == "auth" || r.ServerName == "storage" {
			foundBase++
		}
	}
	if foundBase < 3 {
		t.Errorf("expected 3 base service routes in result, found %d", foundBase)
	}
}

// TestCollectRoutesBaseService_ConflictDetected verifies the end-to-end scenario
// from T03: a plugin that declares "/api" as its route is rejected when checked
// against the base-service-seeded route set. This proves the false-positive fix:
// on a clean init, such a plugin would previously slip through.
func TestCollectRoutesBaseService_ConflictDetected(t *testing.T) {
	pluginDir := t.TempDir()

	// Write a manifest with an api_endpoint that conflicts with Hasura's base route.
	manifestJSON := []byte(`{"name":"bad-plugin","version":"1.0.0","description":"test","category":"utility","license":"MIT","api_endpoints":["api/"]}`)
	pluginSubDir := filepath.Join(pluginDir, "bad-plugin")
	if err := os.MkdirAll(pluginSubDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginSubDir, "plugin.json"), manifestJSON, 0644); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}

	// "bad-plugin" claims "api/" which conflicts with Hasura's base route.
	badManifest := PluginManifest{
		Name:         "bad-plugin",
		APIEndpoints: []string{"api/"},
	}

	// Collect routes, skipping "bad-plugin" itself (as the install path would).
	existingRoutes := collectInstalledPluginRoutes(pluginDir, "bad-plugin")

	err := checkPluginRouteConflict(&badManifest, existingRoutes)
	if err == nil {
		t.Fatal("S01.T03 regression: plugin claiming /api must be rejected — base service route conflict not detected")
	}
	if !strings.Contains(err.Error(), "hasura") && !strings.Contains(err.Error(), "api") {
		t.Errorf("conflict error should mention 'hasura' or 'api'; got: %v", err)
	}

	t.Logf("conflict correctly detected: %v", err)
}

// TestCollectRoutesSkipsOwnPrevBackup: Update() renames the old install to
// "<name>.prev" before reinstalling. Its routes must not be collected as a
// competing plugin's routes, or every update of a routed plugin conflicts
// with itself. A different plugin's routes are still collected.
func TestCollectRoutesSkipsOwnPrevBackup(t *testing.T) {
	pluginDir := t.TempDir()
	write := func(dir, name string) {
		t.Helper()
		p := filepath.Join(pluginDir, dir)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		m := `{"name":"` + name + `","version":"1.0.0","description":"test","category":"test","license":"MIT",` +
			`"apiEndpoints":["https://` + name + `.local.nself.org/"]}`
		if err := os.WriteFile(filepath.Join(p, "plugin.json"), []byte(m), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("claw.prev", "claw")
	write("mux", "mux")

	routes := collectInstalledPluginRoutes(pluginDir, "claw")
	var sawClaw, sawMux bool
	for _, r := range routes {
		if strings.HasPrefix(r.ServerName, "claw") {
			sawClaw = true
		}
		if strings.HasPrefix(r.ServerName, "mux") {
			sawMux = true
		}
	}
	if sawClaw {
		t.Error("routes from claw.prev (the plugin's own backup) must be skipped during its update")
	}
	if !sawMux {
		t.Error("routes from a different plugin must still be collected")
	}
}
