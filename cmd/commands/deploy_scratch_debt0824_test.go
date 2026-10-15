package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDebt0824ClearScratchSymlinkAncestor(t *testing.T) {
	for _, ancestor := range []string{".nself", ".nself/deploy-build"} {
		t.Run(ancestor, func(t *testing.T) {
			project, outside := t.TempDir(), t.TempDir()
			link := filepath.Join(project, ancestor)
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Fatal(err)
			}
			victim := filepath.Join(scratchDirFor(project, "prod"), "keep")
			if err := os.MkdirAll(filepath.Dir(victim), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(victim, []byte("untouched"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := clearScratch(project, "prod"); err == nil {
				t.Error("cleanup accepted a symlinked ancestor")
			}
			if _, err := os.Stat(victim); err != nil {
				t.Fatalf("cleanup deleted through the ancestor: %v", err)
			}
		})
	}
}
