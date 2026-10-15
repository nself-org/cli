package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlanTreeRejectsEscapingLinks(t *testing.T) {
	src, dst := t.TempDir(), filepath.Join(t.TempDir(), "stage")
	if err := os.Symlink(t.TempDir(), filepath.Join(src, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := CopyPlanTree(src, dst); err == nil {
		t.Fatal("accepted a symlink into live state")
	}
}
