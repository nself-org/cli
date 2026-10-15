package portable

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOpenFileChecksConsumedBytes covers edits that preserve every snapshot
// field, both before opening a member and while its stream is being consumed.
func TestOpenFileChecksConsumedBytes(t *testing.T) {
	for _, afterOpen := range []bool{false, true} {
		name := "before open"
		if afterOpen {
			name = "during read"
		}
		t.Run(name, func(t *testing.T) {
			const member = "db/data.dump"
			const original = "verified payload"
			dir := makeBundle(t, map[string]string{member: original})
			r, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(dir, filepath.FromSlash(member))
			mtime := mustStat(t, p).ModTime()
			mutate := func() {
				t.Helper()
				if err := os.WriteFile(p, []byte(strings.Repeat("x", len(original))), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(p, mtime, mtime); err != nil {
					t.Fatal(err)
				}
			}
			if !afterOpen {
				mutate()
			}
			fh, err := r.OpenFile(member)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = fh.Close() }()
			if afterOpen {
				prefix := make([]byte, 3)
				if _, err := io.ReadFull(fh, prefix); err != nil {
					t.Fatal(err)
				}
				mutate()
			}
			_, err = io.ReadAll(fh)
			wantCode(t, err, "E516", ErrChecksum)
			_, again := fh.Read(make([]byte, 1))
			wantCode(t, again, "E516", ErrChecksum)
		})
	}
}

// TestMemberParentHandleSurvivesSwap deterministically performs the attacker
// rename between opening a parent and opening its child. A path-based child
// open would follow the replacement link to the attacker's directory.
func TestMemberParentHandleSurvivesSwap(t *testing.T) {
	const member = "db/data.dump"
	dir := makeBundle(t, map[string]string{member: "verified payload"})
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	parentPath := filepath.Join(dir, "db")
	parent, err := openMemberDirectory(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = parent.Close() }()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "data.dump"), []byte("attacker payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(parentPath, filepath.Join(dir, "saved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, parentPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	fh, err := openMemberAt(parent, "data.dump", false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fh.Close() }()
	b, err := io.ReadAll(fh)
	if err != nil || string(b) != "verified payload" {
		t.Fatalf("pinned parent read %q, %v", b, err)
	}
	if rc, err := r.OpenFile(member); err == nil {
		_ = rc.Close()
		t.Fatal("a fresh walk followed the swapped parent symlink")
	} else {
		wantCode(t, err, "E516", ErrLink)
	}
}

// A link to the very same directory must also be refused, even though its
// target's identity matches the directory that was previously inspected.
func TestMemberAtRefusesSameDirectoryLink(t *testing.T) {
	dir := makeBundle(t, map[string]string{"db/data.dump": "payload"})
	parent, err := openMemberDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = parent.Close() }()
	if err := os.Symlink("db", filepath.Join(dir, "alias")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if fh, err := openMemberAt(parent, "alias", true); err == nil {
		_ = fh.Close()
		t.Fatal("directory component followed a symlink")
	}
}
