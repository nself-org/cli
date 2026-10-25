package portable

// reader_race_test.go — D-0252: a writer racing the reader cannot make it
// accept bytes the manifest does not hash.
//
// Open and Verify hash every byte they read and compare the descriptor to the
// snapshot taken at Open, so neither a same-size in-place edit with the mtime
// put back nor a parent directory swapped for a symlink is accepted.
// OpenFile alone hands out a descriptor whose bytes are not hashed again (its
// documented contract); it is covered by the snapshot checks in
// openfile_test.go.

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestSameSizeEditWithMtimeRestoredIsRefused edits a member in place, keeps
// its size, puts the old mtime back, and checks the stream and Verify still
// fail on the hash.
func TestSameSizeEditWithMtimeRestoredIsRefused(t *testing.T) {
	dir := fixture(t)
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "db", "schema.sql")
	before := mustStat(t, p)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	b[0] ^= 1
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}

	rc, err := r.Open("db/schema.sql")
	if err != nil {
		t.Fatalf("the snapshot cannot tell this edit apart, so Open must hand out the stream: %v", err)
	}
	_, err = io.ReadAll(rc)
	_ = rc.Close()
	wantCode(t, err, "E516", ErrChecksum)
	wantCode(t, r.Verify(), "E516", ErrChecksum)
}

// TestSwappedParentDirectoryIsRefused replaces a verified directory with a
// symlink to a copy holding a same-size file with other bytes.
func TestSwappedParentDirectoryIsRefused(t *testing.T) {
	dir := fixture(t)
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	evil := t.TempDir()
	orig, err := os.ReadFile(filepath.Join(dir, "db", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	orig[0] ^= 1
	for _, name := range []string{"schema.sql", "data.dump"} {
		if err := os.WriteFile(filepath.Join(evil, name), orig, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.RemoveAll(filepath.Join(dir, "db")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(evil, filepath.Join(dir, "db")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	rc, err := r.Open("db/schema.sql")
	if err == nil {
		_, err = io.ReadAll(rc)
		_ = rc.Close()
	}
	wantCode(t, err, "E516", nil)
	if fh, err := r.OpenFile("db/schema.sql"); err == nil {
		_ = fh.Close()
		t.Fatal("OpenFile followed a swapped parent directory")
	}
	wantCode(t, r.Verify(), "E516", nil)
}
