package plugin

// ReverseDependencies tests (D-0325): the exported scan over installed
// manifests, its case rules, skipped directories and name validation.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
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
	writeDepManifest(t, dir, "broken", `{not json`)
	if err := os.MkdirAll(filepath.Join(dir, "no-manifest"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ReverseDependencies(dir, "target")
	var unreadable *UnreadableManifestsError
	if !errors.As(err, &unreadable) || len(unreadable.Dirs) != 1 || unreadable.Dirs[0] != "broken" {
		t.Fatalf("err = %v, want unreadable [broken]", err)
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

// D-0662: an unreadable plugin.json is reported, not skipped; dependents are
// directory names; staging folders and ".prev" backups are not plugins.
func TestReverseDependenciesReportsUnreadableManifests(t *testing.T) {
	dir := t.TempDir()
	base := `{"name":"%s","version":"1.0.0","description":"d","category":"utility","license":"MIT"%s}`
	writeDepManifest(t, dir, "dep-dir", fmt.Sprintf(base, "other-name", `,"dependencies":["target"]`))
	writeDepManifest(t, dir, "broken", `{not json`)
	writeDepManifest(t, dir, ".staging-x", `{not json`)
	writeDepManifest(t, dir, "old.prev", `{not json`)
	if err := os.MkdirAll(filepath.Join(dir, "no-manifest"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ReverseDependencies(dir, "target")
	var unreadable *UnreadableManifestsError
	if !errors.As(err, &unreadable) || len(unreadable.Dirs) != 1 || unreadable.Dirs[0] != "broken" {
		t.Fatalf("err = %v, want unreadable [broken]", err)
	}
	if len(got) != 1 || got[0] != "dep-dir" {
		t.Errorf("dependents = %v, want the directory name [dep-dir]", got)
	}
}

func TestCheckReverseDependenciesNoUnreadable(t *testing.T) {
	dir := t.TempDir()
	writeDepManifest(t, dir, "fine", `{"name":"fine","version":"1.0.0","description":"d","category":"utility","license":"MIT"}`)
	if got, err := checkReverseDependencies(dir, "target"); err != nil || len(got) != 0 {
		t.Errorf("got %v, %v; want none", got, err)
	}
}

// D-0662: plugin remove refuses while another plugin's manifest is unreadable
// (it could depend on the target); --force removes anyway.
func TestRemoveRefusesWhenAnotherManifestIsUnreadable(t *testing.T) {
	dir := t.TempDir()
	writeDepManifest(t, dir, "target", `{"name":"target","version":"1.0.0","description":"d","category":"utility","license":"MIT"}`)
	writeDepManifest(t, dir, "midupdate", `{"name":`)
	err := Remove(context.Background(), &config.Config{}, "target", dir, true, false)
	if err == nil || !strings.Contains(err.Error(), "midupdate") || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("err = %v, want a refusal naming midupdate and --force", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "target")); statErr != nil {
		t.Errorf("target was removed: %v", statErr)
	}
	if err := Remove(context.Background(), &config.Config{}, "target", dir, true, true); err != nil {
		t.Fatalf("forced remove: %v", err)
	}
}
