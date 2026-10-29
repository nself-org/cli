//go:build !windows

package pluginperm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVerifyRefusesUnsafe proves that Verify
// rejects world-writable, group-writable-by-foreign-gid, and foreign-owned path
// components on Unix. Removing the check would make these tests pass when they
// must fail. Windows skips the check by design (see perm_windows.go).
func TestVerifyRefusesUnsafe(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(sub, "bin")
	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Baseline: owned by us, no unsafe bits -> passes.
	if err := Verify(target, root); err != nil {
		t.Fatalf("baseline failed: %v", err)
	}

	// World-writable component -> refused.
	if err := os.Chmod(sub, 0o757); err != nil {
		t.Fatal(err)
	}
	if err := Verify(target, root); err == nil || !strings.Contains(err.Error(), "world-writable") {
		t.Fatalf("world-writable not refused: %v", err)
	}
	if err := os.Chmod(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// Group-writable -> refused even when the directory's gid matches the
	// caller's primary gid. Shared groups such as macOS `staff` would
	// otherwise let other members replace plugin binaries. Regression test
	// for review finding: matching the primary GID does not establish a
	// private group.
	if err := os.Chmod(sub, 0o775); err != nil {
		t.Fatal(err)
	}
	if err := Verify(target, root); err == nil || !strings.Contains(err.Error(), "group-writable") {
		t.Fatalf("same-gid group-writable not refused: %v", err)
	}
	if err := os.Chmod(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// Foreign gid with group-write must still be refused (defense in depth).
	foreignGID := findForeignGID(t)
	if foreignGID != 0 {
		if err := os.Chown(sub, -1, int(foreignGID)); err != nil {
			t.Skipf("cannot chown to foreign gid %d: %v", foreignGID, err)
		}
		if err := os.Chmod(sub, 0o775); err != nil {
			t.Fatal(err)
		}
		if err := Verify(target, root); err == nil || !strings.Contains(err.Error(), "group-writable") {
			t.Fatalf("foreign group-writable not refused: %v", err)
		}
		// Restore for subsequent cases.
		if err := os.Chmod(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(sub, -1, os.Getgid()); err != nil {
			t.Logf("warning: could not restore gid: %v", err)
		}
	}

	// Foreign owner -> refused. Only possible when running as root.
	if os.Getuid() == 0 {
		foreignUID := 65534 // nobody on most Linux/macOS systems
		if err := os.Chown(sub, foreignUID, -1); err != nil {
			t.Skipf("cannot chown to foreign uid %d: %v", foreignUID, err)
		}
		if err := Verify(target, root); err == nil || !strings.Contains(err.Error(), "owned by uid") {
			t.Fatalf("foreign owner not refused: %v", err)
		}
	}
}

// findForeignGID returns a gid that isn't the current effective gid, or 0 if
// none is available. It scans /etc/group via os/user when possible; falling
// back to 0 keeps the caller's skip logic honest.
func findForeignGID(t *testing.T) uint32 {
	t.Helper()
	myGID := uint32(os.Getgid())
	// Try a few well-known gids that exist on most Unix systems and aren't
	// the user's primary group. This avoids importing os/user just for the
	// test and keeps the dependency footprint small.
	candidates := []uint32{1, 2, 99, 65534}
	for _, g := range candidates {
		if g != myGID {
			return g
		}
	}
	return 0
}
