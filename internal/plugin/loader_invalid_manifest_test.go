package plugin

// Purpose: Pin the behaviour of listInstalled when an installed plugin carries
//          an invalid manifest — it must be skipped but WARNED about, never
//          silently dropped.
// Inputs:  A temp plugin dir with one valid and one invalid manifest.
// Outputs: Test results.
// Constraints: Must not depend on the real ~/.nself tree.
// SPORT: list/inventory operations — see loader.go.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeManifest writes a plugin.json for a plugin dir under root.
func writeTestPluginManifest(t *testing.T, root, name string, manifest map[string]any) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest for %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), data, 0o644); err != nil {
		t.Fatalf("write manifest for %s: %v", name, err)
	}
}

func validManifest(name string) map[string]any {
	return map[string]any{
		"name":        name,
		"version":     "1.2.1",
		"description": "A valid test plugin.",
		"category":    "infrastructure",
		"license":     "MIT",
	}
}

// TestListInstalled_InvalidManifestIsSkippedButWarned reproduces the live
// 2026-09-13 defect: the free Task Bundle's `notifications` plugin declares
// status "deprecated" without the `deprecation` block validateManifest
// requires, so its manifest is invalid. `nself plugin install notifications`
// reported success and wrote the directory, and the plugin then vanished from
// `nself plugin list --installed` with no output whatsoever.
//
// The skip itself is correct (one bad manifest must not hide the others). What
// was wrong is that it was silent.
func TestListInstalled_InvalidManifestIsSkippedButWarned(t *testing.T) {
	root := t.TempDir()

	writeTestPluginManifest(t, root, "good-plugin", validManifest("good-plugin"))

	// Exactly the shape that broke: status=deprecated, flat deprecation
	// fields, no `deprecation` block.
	bad := validManifest("notifications")
	bad["status"] = "deprecated"
	bad["deprecated"] = true
	bad["deprecatedSince"] = "1.1.0"
	bad["replacedBy"] = "notify"
	writeTestPluginManifest(t, root, "notifications", bad)

	// Capture stderr for the duration of the call.
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w

	plugins, listErr := listInstalled(root)

	w.Close()
	os.Stderr = origStderr
	var buf [4096]byte
	n, _ := r.Read(buf[:])
	stderr := string(buf[:n])
	r.Close()

	if listErr != nil {
		t.Fatalf("listInstalled returned an error: %v", listErr)
	}

	// The valid plugin must still be listed — a bad neighbour must not hide it.
	if len(plugins) != 1 {
		t.Fatalf("expected exactly 1 listed plugin, got %d: %+v", len(plugins), plugins)
	}
	if plugins[0].Name != "good-plugin" {
		t.Fatalf("expected good-plugin to be listed, got %q", plugins[0].Name)
	}

	// The invalid one must have produced a warning naming it. Before this fix
	// stderr was empty and the plugin simply disappeared.
	if stderr == "" {
		t.Fatal("expected a warning on stderr for the invalid manifest, got nothing — " +
			"the plugin would disappear from `plugin list --installed` silently")
	}
	if !contains(stderr, "notifications") {
		t.Fatalf("warning does not name the offending plugin directory; got: %q", stderr)
	}
}

// TestListInstalled_ValidDeprecatedManifestIsListed proves the fix to the
// manifest itself is the real remedy: with a proper `deprecation` block the
// plugin parses and is listed normally, warning-free.
func TestListInstalled_ValidDeprecatedManifestIsListed(t *testing.T) {
	root := t.TempDir()

	m := validManifest("notifications")
	m["status"] = "deprecated"
	m["deprecation"] = map[string]any{
		"announcedDate":  "2026-05-01",
		"eolDate":        "2027-01-01",
		"replacedBy":     "notify",
		"migrationGuide": "https://nself.org/docs/plugins/notifications",
	}
	writeTestPluginManifest(t, root, "notifications", m)

	plugins, err := listInstalled(root)
	if err != nil {
		t.Fatalf("listInstalled returned an error: %v", err)
	}
	if len(plugins) != 1 || plugins[0].Name != "notifications" {
		t.Fatalf("a validly-deprecated plugin must still be listed; got %+v", plugins)
	}
}
