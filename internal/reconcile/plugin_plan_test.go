package reconcile

import (
	"os"
	"path/filepath"
	"testing"

	nbuild "github.com/nself-org/cli/internal/build"
)

// Canonical compose paths must still read new packages from disposable staging.
func TestPluginPlanShadowReadsStagedPackage(t *testing.T) {
	root := t.TempDir()
	staged := filepath.Join(root, "staged")
	installed := filepath.Join(root, "installed")
	if err := os.MkdirAll(filepath.Join(staged, "fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte("services:\n  fixture:\n    image: fixture:latest\n")
	if err := os.WriteFile(filepath.Join(staged, "fixture", "docker-compose.plugin.yml"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	ov := newOverlay(root, "", &nbuild.PlannedBuild{PluginDir: staged, PluginTargetDir: installed})
	got, found, err := ov.readAbs(filepath.Join(installed, "fixture", "docker-compose.plugin.yml"))
	if err != nil || !found || string(got) != string(body) {
		t.Fatalf("staged package dropped: found=%v err=%v body=%s", found, err, got)
	}
	if _, found, err := ov.readAbs(filepath.Join(installed+"-extra", "fixture", "docker-compose.plugin.yml")); err != nil || found {
		t.Fatalf("remapped a sibling path: found=%v err=%v", found, err)
	}
}
