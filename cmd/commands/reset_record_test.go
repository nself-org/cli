package commands

// Purpose: tests for recordIfExists and the partial-removal wording (D-0375):
// an Lstat failure other than ENOENT is a failure, never "already clean", and
// a failed removal never claims that nothing was removed.
// Constraints: filesystem permissions only in t.TempDir(); skipped as root.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordIfExists_AbsentIsNotAnError(t *testing.T) {
	root := t.TempDir()
	in := []string{"keep.txt"}
	got, err := recordIfExists(in, root, "docker-compose.yml", removeIfExists)
	if err != nil {
		t.Fatalf("ENOENT must not be an error, got %v", err)
	}
	if len(got) != 1 || got[0] != "keep.txt" {
		t.Fatalf("removed changed for an absent path: %v", got)
	}
}

func TestRecordIfExists_RemovesAndRecords(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.yml"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := recordIfExists(nil, root, "a.yml", removeIfExists)
	if err != nil || len(got) != 1 || got[0] != "a.yml" {
		t.Fatalf("got %v, err %v", got, err)
	}
}

func TestRecordIfExists_UninspectablePathIsAnError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "docker-compose.yml"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	called := false
	got, err := recordIfExists([]string{"prior"}, root, "locked/docker-compose.yml", func(string) error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("want an error for a path under an unsearchable directory")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error must not look like ENOENT: %v", err)
	}
	if !strings.Contains(err.Error(), "checking locked/docker-compose.yml") {
		t.Errorf("error should name the path: %v", err)
	}
	if called {
		t.Error("remove ran although the path could not be inspected")
	}
	if len(got) != 1 || got[0] != "prior" {
		t.Errorf("removed changed on error: %v", got)
	}
}

func TestPartialRemovalWording(t *testing.T) {
	cause := errors.New("boom")
	if got := errPartialRemoval("step failed", cause, nil).Error(); !strings.Contains(got, "no files were removed") {
		t.Errorf("pre-deletion failure should say nothing was removed: %q", got)
	}
	got := errRemovalFailed("step failed", cause, nil)
	if !strings.Contains(got.Error(), "some files may have been removed") || strings.Contains(got.Error(), "no files were removed") {
		t.Errorf("removal failure must not claim nothing was removed: %q", got)
	}
	if !errors.Is(got, cause) {
		t.Error("cause not wrapped")
	}
	if g := errRemovalFailed("x", cause, []string{"a", "b/"}).Error(); !strings.Contains(g, "already removed: a, b") {
		t.Errorf("listed paths missing: %q", g)
	}
}
