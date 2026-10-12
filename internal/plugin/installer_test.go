package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// TestUndoInstallRemovesDir: a fresh install with no tables is removed and the
// install lock is released.
func TestUndoInstallRemovesDir(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)
	dir := t.TempDir()
	dest := filepath.Join(dir, "demo")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "plugin.json"), []byte(`{"name":"demo","version":"1.0.0","tables":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UndoInstall(context.Background(), &config.Config{}, "demo", dir); err != nil {
		t.Fatalf("UndoInstall: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("plugin dir still present: %v", err)
	}
	if _, err := os.Stat(installLockPath(dir)); !os.IsNotExist(err) {
		t.Fatalf("install lock left behind: %v", err)
	}
}

// TestUndoInstallRefusesInvalidName: a path-shaped name never reaches the disk.
func TestUndoInstallRefusesInvalidName(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "keep")
	if err := os.MkdirAll(keep, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"../keep", "a/b", "", "Bad Name"} {
		if err := UndoInstall(context.Background(), &config.Config{}, bad, dir); err == nil {
			t.Errorf("UndoInstall(%q) = nil, want an invalid-name error", bad)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("an unrelated dir was touched: %v", err)
	}
}
