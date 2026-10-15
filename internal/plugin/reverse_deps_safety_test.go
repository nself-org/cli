package plugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func TestReverseDependenciesUsesDirectoryIdentity(t *testing.T) {
	dir := t.TempDir()
	writeDepManifest(t, dir, "outsider", `{"name":"planned","version":"1.0.0","description":"d","category":"utility","license":"MIT","dependencies":["target"]}`)
	got, err := ReverseDependencies(dir, "target")
	if err != nil || len(got) != 1 || got[0] != "outsider" {
		t.Fatalf("dependents = %v, %v; want [outsider]", got, err)
	}
}

func TestReverseDependenciesRefusesUnreadableManifest(t *testing.T) {
	for _, manifest := range []string{`{not json`, `{"name":"outsider"}`} {
		t.Run(manifest, func(t *testing.T) {
			dir := t.TempDir()
			writeDepManifest(t, dir, "outsider", manifest)
			if _, err := ReverseDependencies(dir, "target"); err == nil || !strings.Contains(err.Error(), "outsider") {
				t.Fatalf("scan error = %v; want error naming outsider", err)
			}
			writeDepManifest(t, dir, "target", `{"name":"target"}`)
			if err := Remove(context.Background(), &config.Config{}, "target", dir, true, false); err == nil {
				t.Fatal("plugin removal accepted an unreadable outside manifest")
			}
			if _, err := os.Stat(filepath.Join(dir, "target")); err != nil {
				t.Fatalf("target changed despite failed dependency scan: %v", err)
			}
		})
	}
}
