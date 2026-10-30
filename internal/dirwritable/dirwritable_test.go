package dirwritable

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFirstUnwritableCleanTree(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := FirstUnwritable(filepath.Dir(dir), dir); got != "" {
		t.Fatalf("FirstUnwritable = %q, want none", got)
	}
}

func TestFirstUnwritableReportsLockedDirectories(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions are not enforced here")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "out")
	locked := filepath.Join(dir, "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	if got := FirstUnwritable(root, dir); got != locked {
		t.Fatalf("FirstUnwritable = %q, want %q", got, locked)
	}
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	if got := FirstUnwritable(root, dir); got != root {
		t.Fatalf("FirstUnwritable = %q, want the parent %q", got, root)
	}
}

func TestFirstUnwritableUnreadableTree(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "missing")
	if got := FirstUnwritable(root, missing); got != missing {
		t.Fatalf("FirstUnwritable = %q, want %q for an unreadable tree", got, missing)
	}
}
