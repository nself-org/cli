package portable

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fixture(t *testing.T) string {
	return makeBundle(t, map[string]string{
		"db/data.dump":        "PGDMP-bytes-SECRETVALUE-123",
		"db/schema.sql":       "create table notes();",
		"storage/default/a.t": "object",
	})
}

func TestReaderUnknownMajor(t *testing.T) {
	for _, v := range []string{"2", "0", "10", "01", "1.", "", "1.x", "99999999999999999999"} {
		dir := fixture(t)
		editManifest(t, dir, func(m map[string]any) { m["schema_version"] = v })
		_, err := Open(dir)
		wantCode(t, err, "E515", ErrUnknownMajor)
	}
	dir := fixture(t)
	editManifest(t, dir, func(m map[string]any) { m["schema_version"] = 2 }) // number, not string
	_, err := Open(dir)
	wantCode(t, err, "E515", ErrManifest)

	dir = fixture(t)
	editManifest(t, dir, func(m map[string]any) { m["_format"] = "something-else" })
	_, err = Open(dir)
	wantCode(t, err, "E515", ErrUnknownMajor)

	// A later minor of major 1 is read.
	dir = fixture(t)
	editManifest(t, dir, func(m map[string]any) { m["schema_version"] = "1.4"; m["added_later"] = "x" })
	if _, err := Open(dir); err != nil {
		t.Fatalf("1.4 must open: %v", err)
	}
}

func TestReaderIntegrityFailures(t *testing.T) {
	t.Run("flipped byte names the file", func(t *testing.T) {
		dir := fixture(t)
		p := filepath.Join(dir, "db", "data.dump")
		b, _ := os.ReadFile(p)
		b[3] ^= 0xff
		_ = os.WriteFile(p, b, 0o600)
		_, err := Open(dir)
		wantCode(t, err, "E516", ErrChecksum)
		if !strings.Contains(err.Error(), "db/data.dump") {
			t.Errorf("error must name the file: %v", err)
		}
		if strings.Contains(err.Error(), "SECRETVALUE") {
			t.Error("error leaked bundle content")
		}
	})
	t.Run("missing file", func(t *testing.T) {
		dir := fixture(t)
		_ = os.Remove(filepath.Join(dir, "db", "schema.sql"))
		_, err := Open(dir)
		wantCode(t, err, "E516", ErrMissing)
		if !strings.Contains(err.Error(), "db/schema.sql") {
			t.Errorf("error must name the file: %v", err)
		}
	})
	t.Run("truncated", func(t *testing.T) {
		dir := fixture(t)
		_ = os.Truncate(filepath.Join(dir, "db", "data.dump"), 5)
		_, err := Open(dir)
		wantCode(t, err, "E516", ErrSize)
	})
	t.Run("longer than declared", func(t *testing.T) {
		dir := fixture(t)
		f, _ := os.OpenFile(filepath.Join(dir, "db", "data.dump"), os.O_APPEND|os.O_WRONLY, 0)
		_, _ = f.WriteString("more")
		_ = f.Close()
		_, err := Open(dir)
		wantCode(t, err, "E516", ErrSize)
	})
	t.Run("unlisted file", func(t *testing.T) {
		dir := fixture(t)
		_ = os.WriteFile(filepath.Join(dir, "db", "extra.sql"), []byte("drop database x"), 0o600)
		_, err := Open(dir)
		wantCode(t, err, "E516", ErrUnlisted)
	})
	t.Run("truncated manifest", func(t *testing.T) {
		dir := fixture(t)
		p := filepath.Join(dir, ManifestName)
		b, _ := os.ReadFile(p)
		_ = os.WriteFile(p, b[:len(b)/2], 0o600)
		_, err := Open(dir)
		wantCode(t, err, "E515", ErrManifest)
	})
	t.Run("trailing data after manifest", func(t *testing.T) {
		dir := fixture(t)
		p := filepath.Join(dir, ManifestName)
		b, _ := os.ReadFile(p)
		_ = os.WriteFile(p, append(b, []byte(`{"x":1}`)...), 0o600)
		_, err := Open(dir)
		wantCode(t, err, "E515", ErrManifest)
	})
	t.Run("no manifest", func(t *testing.T) {
		_, err := Open(t.TempDir())
		wantCode(t, err, "E515", ErrManifest)
	})
	t.Run("not a directory", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "file")
		_ = os.WriteFile(f, []byte("x"), 0o600)
		_, err := Open(f)
		wantCode(t, err, "E515", ErrManifest)
	})
}

func TestReaderRefusesUnsafeAndDuplicateMembers(t *testing.T) {
	cases := []struct {
		name     string
		paths    []string
		sentinel error
	}{
		{"parent traversal", []string{"../outside"}, ErrUnsafePath},
		{"nested traversal", []string{"db/../../outside"}, ErrUnsafePath},
		{"absolute", []string{"/etc/passwd"}, ErrUnsafePath},
		{"windows drive", []string{"C:/Windows/x"}, ErrUnsafePath},
		{"windows drive relative", []string{"C:x"}, ErrUnsafePath},
		{"backslash", []string{`db\..\x`}, ErrUnsafePath},
		{"unc", []string{`\\host\share\x`}, ErrUnsafePath},
		{"empty segment", []string{"db//x"}, ErrUnsafePath},
		{"dot segment", []string{"db/./x"}, ErrUnsafePath},
		{"nul byte", []string{"db/x\x00.sql"}, ErrUnsafePath},
		{"device name", []string{"db/NUL.txt"}, ErrUnsafePath},
		{"empty", []string{""}, ErrUnsafePath},
		{"lists the manifest", []string{ManifestName}, ErrUnsafePath},
		{"duplicate", []string{"db/a", "db/a"}, ErrDuplicate},
		{"case duplicate", []string{"db/a", "db/A"}, ErrDuplicate},
	}
	for _, c := range cases {
		dir := fixture(t)
		editManifest(t, dir, func(m map[string]any) {
			var es []map[string]any
			for _, p := range c.paths {
				es = append(es, fileEntry(p, zeroSum, 0))
			}
			setFiles(m, es...)
		})
		_, err := Open(dir)
		if c.name == "lists the manifest" {
			wantCode(t, err, "E516", nil)
			continue
		}
		t.Run(c.name, func(t *testing.T) { wantCode(t, err, "E516", c.sentinel) })
	}
	// A path that is a file and also a directory prefix of another member.
	dir := fixture(t)
	editManifest(t, dir, func(m map[string]any) {
		setFiles(m, fileEntry("db", zeroSum, 0), fileEntry("db/x", zeroSum, 0))
	})
	_, err := Open(dir)
	wantCode(t, err, "E516", nil)
	// Nothing outside the bundle is touched or read for a traversal path.
	outside := filepath.Join(filepath.Dir(dir), "outside")
	if _, err := os.Stat(outside); err == nil {
		t.Error("traversal created a file outside the bundle")
	}
}

func TestReaderRefusesLinks(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "host-secret")
	_ = os.WriteFile(secret, []byte("per-host-secret"), 0o600)

	t.Run("symlink member to outside file", func(t *testing.T) {
		dir := fixture(t)
		p := filepath.Join(dir, "db", "schema.sql")
		_ = os.Remove(p)
		if err := os.Symlink(secret, p); err != nil {
			t.Skip("symlinks unavailable")
		}
		_, err := Open(dir)
		wantCode(t, err, "E516", ErrLink)
	})
	t.Run("symlink to outside file with matching hash and size", func(t *testing.T) {
		dir := makeBundle(t, map[string]string{"db/x": "per-host-secret"})
		p := filepath.Join(dir, "db", "x")
		_ = os.Remove(p)
		if err := os.Symlink(secret, p); err != nil {
			t.Skip("symlinks unavailable")
		}
		_, err := Open(dir)
		wantCode(t, err, "E516", ErrLink)
	})
	t.Run("symlinked parent directory", func(t *testing.T) {
		dir := fixture(t)
		other := t.TempDir()
		_ = os.WriteFile(filepath.Join(other, "data.dump"), []byte("PGDMP-bytes-SECRETVALUE-123"), 0o600)
		_ = os.RemoveAll(filepath.Join(dir, "db"))
		if err := os.Symlink(other, filepath.Join(dir, "db")); err != nil {
			t.Skip("symlinks unavailable")
		}
		_, err := Open(dir)
		if err == nil {
			t.Fatal("a symlinked directory must be refused")
		}
		wantCode(t, err, "E516", nil)
	})
	t.Run("stray symlink", func(t *testing.T) {
		dir := fixture(t)
		if err := os.Symlink(secret, filepath.Join(dir, "loot")); err != nil {
			t.Skip("symlinks unavailable")
		}
		_, err := Open(dir)
		wantCode(t, err, "E516", ErrLink)
	})
	t.Run("hard link member", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("hard-link counts are not read on Windows")
		}
		dir := makeBundle(t, map[string]string{"db/x": "per-host-secret"})
		p := filepath.Join(dir, "db", "x")
		_ = os.Remove(p)
		_ = os.WriteFile(filepath.Join(filepath.Dir(secret), "twin"), []byte("per-host-secret"), 0o600)
		if err := os.Link(filepath.Join(filepath.Dir(secret), "twin"), p); err != nil {
			t.Skip("hard links unavailable")
		}
		_, err := Open(dir)
		wantCode(t, err, "E516", ErrLink)
	})
	t.Run("manifest is a symlink", func(t *testing.T) {
		dir := fixture(t)
		p := filepath.Join(dir, ManifestName)
		b, _ := os.ReadFile(p)
		other := filepath.Join(t.TempDir(), "m.json")
		_ = os.WriteFile(other, b, 0o600)
		_ = os.Remove(p)
		if err := os.Symlink(other, p); err != nil {
			t.Skip("symlinks unavailable")
		}
		_, err := Open(dir)
		wantCode(t, err, "E516", ErrLink)
	})
}

func TestReaderLimits(t *testing.T) {
	t.Run("declared size over the per-file limit", func(t *testing.T) {
		dir := fixture(t)
		editManifest(t, dir, func(m map[string]any) {
			setFiles(m, fileEntry("db/data.dump", zeroSum, 1<<62))
		})
		_, err := Open(dir)
		wantCode(t, err, "E516", ErrSize)
	})
	t.Run("declared total over the limit", func(t *testing.T) {
		dir := fixture(t)
		_, err := OpenWithLimits(dir, Limits{MaxTotalBytes: 10})
		wantCode(t, err, "E516", ErrSize)
	})
	t.Run("too many files", func(t *testing.T) {
		dir := fixture(t)
		_, err := OpenWithLimits(dir, Limits{MaxFiles: 2})
		wantCode(t, err, "E516", ErrSize)
	})
	t.Run("manifest too large", func(t *testing.T) {
		dir := fixture(t)
		_, err := OpenWithLimits(dir, Limits{MaxManifestBytes: 20})
		wantCode(t, err, "E516", ErrSize)
	})
	t.Run("file inflated past its declared length is never read to the end", func(t *testing.T) {
		dir := fixture(t)
		r, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, "db", "data.dump")
		huge := make([]byte, 1<<20)
		_ = os.WriteFile(p, huge, 0o600)
		rc, err := r.Open("db/data.dump")
		if err == nil {
			_, err = io.Copy(io.Discard, rc)
			_ = rc.Close()
		}
		wantCode(t, err, "E516", ErrSize)
	})
}

func TestReaderDetectsChangeAfterOpen(t *testing.T) {
	dir := fixture(t)
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "db", "schema.sql")
	b, _ := os.ReadFile(p)
	b[0] ^= 1 // same length, different bytes
	_ = os.WriteFile(p, b, 0o600)
	rc, err := r.Open("db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(rc)
	_ = rc.Close()
	wantCode(t, err, "E516", ErrChecksum)
	wantCode(t, r.Verify(), "E516", ErrChecksum)
	if _, err := r.Open("db/not-listed"); err == nil {
		t.Error("an unlisted member must not open")
	}
	if _, err := r.Path("../x"); err == nil {
		t.Error("Path must refuse an unlisted name")
	}
}
