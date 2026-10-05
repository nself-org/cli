package portable

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestOpenFileReturnsTheVerifiedFile: the descriptor reads the verified bytes,
// and a member swapped after Open is refused instead of handed to the caller.
func TestOpenFileReturnsTheVerifiedFile(t *testing.T) {
	const member = "db/data.dump"
	body := "PGDMP-bytes-SECRETVALUE-123"

	t.Run("unchanged", func(t *testing.T) {
		r, err := Open(fixture(t))
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			fh, err := r.OpenFile(member)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(fh)
			_ = fh.Close()
			if string(b) != body {
				t.Fatalf("OpenFile read %q", b)
			}
		}
		if err := r.Verify(); err != nil {
			t.Fatalf("Verify on an untouched bundle: %v", err)
		}
		if _, err := r.OpenFile("db/not-listed"); err == nil {
			t.Error("an unlisted member must not open")
		}
	})

	t.Run("swapped for a symlink", func(t *testing.T) {
		dir := fixture(t)
		r, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(t.TempDir(), "elsewhere")
		_ = os.WriteFile(target, []byte(body), 0o600) // same bytes: only the link tells
		p := filepath.Join(dir, "db", "data.dump")
		_ = os.Remove(p)
		if err := os.Symlink(target, p); err != nil {
			t.Skip("symlinks unavailable")
		}
		fh, err := r.OpenFile(member)
		if err == nil {
			_ = fh.Close()
			t.Fatal("OpenFile followed a symlink swapped in after Open")
		}
		wantCode(t, err, "E516", ErrLink)
		if rc, err := r.Open(member); err == nil {
			_ = rc.Close()
			t.Fatal("Open followed the symlink")
		}
	})

	t.Run("swapped for a different file with the same bytes", func(t *testing.T) {
		dir := fixture(t)
		r, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, "db", "data.dump")
		other := filepath.Join(dir, "db", "other.tmp")
		if err := os.WriteFile(other, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(other, p); err != nil {
			t.Skipf("cannot replace a file here: %v", err)
		}
		fh, err := r.OpenFile(member)
		if err == nil {
			_ = fh.Close()
			t.Fatal("OpenFile accepted a different file with equal bytes")
		}
		wantCode(t, err, "E516", ErrChanged)
		wantCode(t, r.Verify(), "E516", ErrChanged)
	})

	t.Run("rewritten in place with a different mtime", func(t *testing.T) {
		dir := fixture(t)
		r, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, "db", "data.dump")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		future := mustStat(t, p).ModTime().Add(7 * time.Second)
		if err := os.Chtimes(p, future, future); err != nil {
			t.Fatal(err)
		}
		fh, err := r.OpenFile(member)
		if err == nil {
			_ = fh.Close()
			t.Fatal("OpenFile accepted a member whose mtime moved")
		}
		wantCode(t, err, "E516", ErrChanged)
	})
}

func mustStat(t *testing.T, p string) os.FileInfo {
	t.Helper()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi
}

// TestStraysCountDirectories: empty directories count toward MaxFiles, so a
// tree of them cannot make the walk unbounded.
func TestStraysCountDirectories(t *testing.T) {
	dir := makeBundle(t, map[string]string{})
	deep := dir
	for i := 0; i < 20; i++ {
		deep = filepath.Join(deep, "d")
	}
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := OpenWithLimits(dir, Limits{MaxFiles: 5})
	wantCode(t, err, "E516", ErrSize)
	if _, err := Open(dir); err != nil {
		t.Fatalf("empty directories are allowed under the default limit: %v", err)
	}
}
