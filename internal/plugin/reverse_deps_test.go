package plugin

// ReverseDependencies tests (D-0325): the exported scan over installed
// manifests, its case rules, skipped directories and name validation.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func writeDepManifest(t *testing.T, pluginDir, dir, manifest string) {
	t.Helper()
	d := filepath.Join(pluginDir, dir)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReverseDependencies(t *testing.T) {
	dir := t.TempDir()
	base := `{"name":"%s","version":"1.0.0","description":"d","category":"utility","license":"MIT"%s}`
	mk := func(name, extra string) string { return fmt.Sprintf(base, name, extra) }
	writeDepManifest(t, dir, "target", mk("target", ""))
	writeDepManifest(t, dir, "dep-one", mk("dep-one", `,"dependencies":["Target"]`))
	writeDepManifest(t, dir, "dep-two", mk("dep-two", `,"dependencies":["other","target"]`))
	writeDepManifest(t, dir, "bystander", mk("bystander", `,"dependencies":["other"]`))
	if err := os.MkdirAll(filepath.Join(dir, "no-manifest"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ReverseDependencies(dir, "target")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	if len(got) != 2 || got[0] != "dep-one" || got[1] != "dep-two" {
		t.Errorf("dependents = %v, want [dep-one dep-two]", got)
	}
}

func TestReverseDependenciesEdges(t *testing.T) {
	if got, err := ReverseDependencies(filepath.Join(t.TempDir(), "absent"), "target"); err != nil || len(got) != 0 {
		t.Errorf("missing plugin dir: got %v, %v; want none", got, err)
	}
	for _, bad := range []string{"", "../x", "a/b", "Bad Name"} {
		if _, err := ReverseDependencies(t.TempDir(), bad); err == nil {
			t.Errorf("name %q accepted", bad)
		}
	}
}
