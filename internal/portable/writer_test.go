package portable

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriterModesAndRefusals(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "b")
	w, err := NewWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	w.Now = fixedNow
	if _, err := w.WriteFile("db/data.dump", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"../x", "/abs", "db/../../x", `a\b`, "C:/x", "db/data.dump", "DB/DATA.DUMP", ManifestName, "db/data.dump/inner", "db"} {
		if _, err := w.WriteFile(rel, strings.NewReader("y")); err == nil {
			t.Errorf("WriteFile(%q) must fail", rel)
		}
	}
	if _, err := w.Finish(baseManifest()); err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteFile("late", strings.NewReader("y")); err == nil {
		t.Error("write after Finish must fail")
	}
	if runtime.GOOS != "windows" {
		for p, want := range map[string]os.FileMode{dir: 0o700, filepath.Join(dir, "db"): 0o700,
			filepath.Join(dir, "db", "data.dump"): 0o600, filepath.Join(dir, ManifestName): 0o600} {
			fi, err := os.Stat(p)
			if err != nil || fi.Mode().Perm() != want {
				t.Errorf("%s mode %v, want %v (err %v)", p, fi.Mode().Perm(), want, err)
			}
		}
	}
	if _, err := Open(dir); err != nil {
		t.Fatalf("Open after refusals: %v", err)
	}
	if _, err := NewWriter(dir); err == nil {
		t.Error("NewWriter must refuse a non-empty directory")
	}
}

type failingReader struct{ n int }

func (f *failingReader) Read(p []byte) (int, error) {
	if f.n == 0 {
		return 0, errors.New("boom")
	}
	f.n--
	p[0] = 'x'
	return 1, nil
}

func TestWriterRemovesPartialFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "b")
	w, _ := NewWriter(dir)
	if _, err := w.WriteFile("db/part", io.MultiReader(&failingReader{n: 3})); err == nil {
		t.Fatal("want copy error")
	}
	if _, err := os.Stat(filepath.Join(dir, "db", "part")); !os.IsNotExist(err) {
		t.Error("partial file must be removed")
	}
}

func TestWriterRefusesSymlinkParent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "b")
	w, _ := NewWriter(dir)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "db")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if _, err := w.WriteFile("db/x", strings.NewReader("y")); err == nil {
		t.Fatal("must refuse a symlinked parent")
	}
	if ents, _ := os.ReadDir(outside); len(ents) != 0 {
		t.Error("write escaped through the symlink")
	}
}

func TestFinishRejectsInvalidManifest(t *testing.T) {
	w, _ := NewWriter(filepath.Join(t.TempDir(), "b"))
	m := baseManifest()
	m.DB.Tables[0].Hash = "not-decimal"
	if _, err := w.Finish(m); !errors.Is(err, ErrManifest) {
		t.Fatalf("want ErrManifest, got %v", err)
	}
	m = baseManifest()
	m.CreatedAt = "yesterday"
	if _, err := w.Finish(m); !errors.Is(err, ErrManifest) {
		t.Fatalf("want ErrManifest for created_at, got %v", err)
	}
	m = baseManifest()
	m.SchemaVersion = "2"
	if _, err := w.Finish(m); err == nil {
		t.Fatal("Finish must not write a non-v1 manifest")
	}
}

func TestCheckMemberAndStorageMember(t *testing.T) {
	good := []string{"manifest-x", "db/data.dump", "storage/default/a b/ключ+%25.png", "a/b/c.d/e"}
	for _, p := range good {
		if err := CheckMember(p); err != nil {
			t.Errorf("CheckMember(%q): %v", p, err)
		}
	}
	bad := []string{"", "/", "a/", "a//b", "..", "../a", "a/..", "a/./b", `a\b`, "c:", "a/b:c", "a/b.", "a/b ", "a/con",
		"a/COM1.txt", "a/aux.tar.gz", "a/\x01", "a/\x7f", "a/\xff\xfe", strings.Repeat("a", 300), strings.Repeat("a/", 3000)}
	for _, p := range bad {
		if CheckMember(p) == nil {
			t.Errorf("CheckMember(%q) must fail", p)
		}
	}
	keys := []string{"a.png", "dir/file name.txt", "ключ/файл+1%.txt", "../../etc/passwd", "/leading", "trail/", "a//b",
		`back\slash`, "c:/win", "NUL", "con.txt", "x.", "dot/./dot", "..", ".", "star*?\"<>|", "tab\tkey", "emoji-\U0001F600"}
	seen := map[string]string{}
	for _, k := range keys {
		m, err := StorageMember("bkt", k)
		if err != nil {
			t.Errorf("StorageMember(%q): %v", k, err)
			continue
		}
		if err := CheckMember(m); err != nil {
			t.Errorf("StorageMember(%q) = %q is unsafe: %v", k, m, err)
		}
		if !strings.HasPrefix(m, "storage/bkt/") {
			t.Errorf("StorageMember(%q) = %q escapes the bucket", k, m)
		}
		if prev, dup := seen[m]; dup {
			t.Errorf("StorageMember collides: %q and %q -> %q", prev, k, m)
		}
		seen[m] = k
	}
	if m, _ := StorageMember("bkt", "ключ/файл+1%.txt"); m != "storage/bkt/ключ/файл+1%25.txt" {
		t.Errorf("unicode and + stay readable, %% is escaped; got %q", m)
	}
	for _, b := range []string{"", "a/b", `a\b`, "..", "a:b"} {
		if _, err := StorageMember(b, "k"); err == nil {
			t.Errorf("bucket %q must be refused", b)
		}
	}
}
