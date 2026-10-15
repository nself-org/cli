package build

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExpectTransactionExternalSymlinkChmodRollback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require privileges")
	}
	dir, external := t.TempDir(), t.TempDir()
	target := filepath.Join(external, "env")
	if err := os.WriteFile(target, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, ".env")); err != nil {
		t.Fatal(err)
	}
	sink := newExpectSink(dir, &PlannedBuild{Modes: map[string]fs.FileMode{".env": 0600}})
	if err := sink.Chmod(".env", 0600); err != nil {
		t.Fatal(err)
	}
	if err := sink.commit(func() error {
		assertArtifact(t, target, "original", 0600)
		return errors.New("reject")
	}); err == nil {
		t.Fatal("accepted rejected chmod")
	}
	assertArtifact(t, target, "original", 0644)
	if err := sink.commit(); err != nil {
		t.Fatal(err)
	}
	assertArtifact(t, target, "original", 0600)
}
