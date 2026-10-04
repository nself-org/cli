package cmdregistry

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "regenerate testdata/registry.golden.json")

// TestGoldenRegistry pins the registry bytes: renaming or reordering any field
// fails it. Run with -update to regenerate after an intended change.
func TestGoldenRegistry(t *testing.T) {
	got := mustMarshal(t, mustBuild(t, fixtureRoot(false), true))
	path := filepath.Join("testdata", "registry.golden.json")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("registry differs from %s (run with -update if intended)\n--- got ---\n%s", path, got)
	}
}
