package sharedproxy

// Tests for plainDirs and mkdirPrivate: symlink and file ancestors, tightened
// modes, and filesystem errors that are not "missing". Helpers are in
// quarantine_edge_test.go.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlainDirs(t *testing.T) {
	top := t.TempDir()
	root := openRoot(t, top)
	if err := os.MkdirAll(filepath.Join(top, "real", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(top, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	linked := os.Symlink(filepath.Join(top, "real"), filepath.Join(top, "link")) == nil

	if err := plainDirs(root, filepath.Join("real", "deep")); err != nil {
		t.Errorf("real directories pass: %v", err)
	}
	if err := plainDirs(root, filepath.Join("real", "missing", "x")); err != nil {
		t.Errorf("a missing tail passes: %v", err)
	}
	if err := plainDirs(root, "."); err != nil {
		t.Errorf("the root itself passes: %v", err)
	}
	if err := plainDirs(root, filepath.Join("file", "x")); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("a file on the way: %v", err)
	}
	if linked {
		if err := plainDirs(root, filepath.Join("link", "deep")); err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Errorf("a symlink ancestor: %v", err)
		}
	}
}

func TestDirHelpersSurfaceFilesystemErrors(t *testing.T) {
	skipIfRoot(t)
	top := t.TempDir()
	root := openRoot(t, top)
	locked := filepath.Join(top, "locked")
	if err := os.MkdirAll(filepath.Join(locked, "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	inner := filepath.Join("locked", "inner")
	if err := plainDirs(root, inner); err == nil {
		t.Error("plainDirs must report an Lstat failure that is not 'missing'")
	}
	if err := mkdirPrivate(root, inner, "shared"); err == nil {
		t.Error("mkdirPrivate must report an Lstat failure that is not 'missing'")
	}

	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	if err := mkdirPrivate(root, filepath.Join("locked", "new"), "shared"); err == nil {
		t.Error("mkdirPrivate must report a failed Mkdir")
	}
	if _, err := os.Stat(filepath.Join(locked, "new")); err == nil {
		t.Error("the directory must not exist after a failed Mkdir")
	}
}

func TestMkdirPrivateTightensExistingDirs(t *testing.T) {
	top := t.TempDir()
	root := openRoot(t, top)
	if err := os.MkdirAll(filepath.Join(top, "shared", "q", "p"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := mkdirPrivate(root, filepath.Join("shared", "q", "p"), "shared"); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{filepath.Join("shared", "q"), filepath.Join("shared", "q", "p")} {
		if fi, err := os.Stat(filepath.Join(top, rel)); err != nil || fi.Mode().Perm() != 0o700 {
			t.Errorf("%s must be tightened to 0700: %v %v", rel, fi, err)
		}
	}
	if fi, _ := os.Stat(filepath.Join(top, "shared")); fi.Mode().Perm() != 0o755 {
		t.Errorf("shared keeps its mode, got %v", fi.Mode().Perm())
	}
}
