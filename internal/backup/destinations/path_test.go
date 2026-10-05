package destinations

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newPath(t *testing.T) (Destination, string) {
	t.Helper()
	skipWindows(t)
	dir := filepath.Join(t.TempDir(), "store")
	d, err := Parse("path://"+dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	return d, dir
}

func TestPathDestinationRoundTrip(t *testing.T) {
	old := syscallUmask(0)
	defer syscallUmask(old)
	d, dir := newPath(t)
	src := writeFile(t, t.TempDir(), "a.dump", "hello backup")
	if err := d.Put(t.Context(), src, "db/a.dump"); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]os.FileMode{dir: 0o700, filepath.Join(dir, "db"): 0o700, filepath.Join(dir, "db", "a.dump"): 0o600} {
		if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s mode %v (want %v) err %v", p, fi.Mode().Perm(), want, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "db", "a.dump.tmp")); err == nil {
		t.Error("tmp file left behind")
	}
	writeFile(t, dir, "db/b.dump.tmp", "partial")
	objs, err := d.List(t.Context(), "")
	if err != nil || len(objs) != 1 || objs[0].Key != "db/a.dump" || objs[0].Size != 12 {
		t.Fatalf("list %+v %v", objs, err)
	}
	out := filepath.Join(t.TempDir(), "o")
	if err := d.Get(t.Context(), "db/a.dump", out); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); string(b) != "hello backup" {
		t.Fatalf("got %q", b)
	}
	if fi, _ := os.Stat(out); fi.Mode().Perm() != 0o600 {
		t.Errorf("downloaded mode %v", fi.Mode().Perm())
	}
	rc, err := d.(Opener).Open(t.Context(), "db/a.dump")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(rc); string(b) != "hello backup" {
		t.Fatalf("open %q", b)
	}
	_ = rc.Close()
	if objs, _ := d.List(t.Context(), "zzz"); len(objs) != 0 {
		t.Fatalf("prefix filter: %+v", objs)
	}
}

func TestPathDestinationListMissingDir(t *testing.T) {
	d, _ := newPath(t)
	if objs, err := d.List(t.Context(), ""); err != nil || len(objs) != 0 {
		t.Fatalf("%v %v", objs, err)
	}
}

func TestPathDestinationRejectsTraversal(t *testing.T) {
	skipWindows(t)
	for _, uri := range []string{"path://", "path://rel/dir", "path:///a/../b", "path:///a/..", "path://\x00"} {
		if _, err := Parse(uri, nil); err == nil {
			t.Errorf("Parse(%q) accepted", uri)
		}
	}
	d, dir := newPath(t)
	src := writeFile(t, t.TempDir(), "f", "x")
	for _, k := range []string{"../escape", "a/../../escape", "/abs", "", "a//b", "./a", "a/", `a\b`} {
		if err := d.Put(t.Context(), src, k); err == nil {
			t.Errorf("Put key %q accepted", k)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape")); err == nil {
		t.Fatal("file written outside the directory")
	}
}

func TestPathDestinationSymlinks(t *testing.T) {
	d, dir := newPath(t)
	outside := t.TempDir()
	victim := writeFile(t, outside, "victim", "original")
	src := writeFile(t, t.TempDir(), "f", "new content")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// final component is a symlink to a file outside
	if err := os.Symlink(victim, filepath.Join(dir, "link.dump")); err != nil {
		t.Fatal(err)
	}
	if err := d.Put(t.Context(), src, "link.dump"); err == nil {
		t.Error("Put followed a final-component symlink")
	}
	if err := d.Get(t.Context(), "link.dump", filepath.Join(t.TempDir(), "o")); err == nil {
		t.Error("Get followed a final-component symlink")
	}
	// a directory symlink that leaves the destination
	if err := os.Symlink(outside, filepath.Join(dir, "esc")); err != nil {
		t.Fatal(err)
	}
	if err := d.Put(t.Context(), src, "esc/x.dump"); err == nil {
		t.Error("Put followed a directory symlink out of the destination")
	}
	// a stale <key>.tmp symlink must be replaced, not written through
	if err := os.Symlink(victim, filepath.Join(dir, "k.dump.tmp")); err != nil {
		t.Fatal(err)
	}
	if err := d.Put(t.Context(), src, "k.dump"); err != nil {
		t.Fatalf("Put over stale tmp symlink: %v", err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "original" {
		t.Fatalf("symlink target overwritten: %q", b)
	}
	objs, _ := d.List(t.Context(), "")
	for _, o := range objs {
		if strings.Contains(o.Key, "link") || strings.Contains(o.Key, "esc") {
			t.Errorf("List returned symlink %q", o.Key)
		}
	}
}

func TestPathDestinationOverwritesAtomically(t *testing.T) {
	d, dir := newPath(t)
	src := writeFile(t, t.TempDir(), "f", "v1")
	if err := d.Put(t.Context(), src, "k"); err != nil {
		t.Fatal(err)
	}
	src2 := writeFile(t, t.TempDir(), "f", "version-two")
	if err := d.Put(t.Context(), src2, "k"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "k")); string(b) != "version-two" {
		t.Fatalf("got %q", b)
	}
	if err := d.Put(t.Context(), filepath.Join(t.TempDir(), "missing"), "k2"); err == nil {
		t.Fatal("missing source accepted")
	}
}
