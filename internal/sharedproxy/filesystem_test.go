package sharedproxy

// Tests for the filesystem helpers: sameDir, Restore, mkdirPrivate and Write.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSameDir(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	if !sameDir(dir, dir+string(filepath.Separator)) {
		t.Error("a trailing separator should not matter")
	}
	if sameDir(dir, other) {
		t.Error("two distinct directories reported as the same")
	}
	if sameDir(dir, filepath.Join(dir, "missing")) {
		t.Error("a missing directory reported as the same")
	}
	if runtime.GOOS == "windows" {
		return
	}
	link := filepath.Join(other, "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if !sameDir(dir, link) {
		t.Error("a symlink to the directory should be the same directory")
	}
}

func TestRestore(t *testing.T) {
	owner := t.TempDir()
	reg := Registration{Project: "demo", Files: []string{"nginx/sites/a.conf", "nginx/sites/b.conf", "../escape"}}

	if back, err := Restore(owner, reg); err != nil || back != nil {
		t.Fatalf("nothing quarantined: got %v, %v", back, err)
	}

	base := quarantineBase(owner, "demo")
	for _, f := range []string{"nginx/sites/a.conf", "nginx/sites/b.conf"} {
		p := filepath.Join(base, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("quarantined "+f), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// b.conf was rebuilt since: the new file wins and the stale copy goes.
	newer := filepath.Join(owner, "nginx", "sites", "b.conf")
	if err := os.MkdirAll(filepath.Dir(newer), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte("rebuilt"), 0o644); err != nil {
		t.Fatal(err)
	}

	back, err := Restore(owner, reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != 1 || back[0] != "nginx/sites/a.conf" {
		t.Fatalf("restored %v, want only a.conf", back)
	}
	if got, _ := os.ReadFile(filepath.Join(owner, "nginx", "sites", "a.conf")); string(got) != "quarantined nginx/sites/a.conf" {
		t.Errorf("a.conf content %q", got)
	}
	if got, _ := os.ReadFile(newer); string(got) != "rebuilt" {
		t.Errorf("the rebuilt file was overwritten: %q", got)
	}
	if _, err := os.Lstat(base); !os.IsNotExist(err) {
		t.Errorf("the quarantine directory should be pruned, Lstat err = %v", err)
	}
}

func TestMkdirPrivate(t *testing.T) {
	top := t.TempDir()
	if err := os.Mkdir(filepath.Join(top, "shared"), 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(top)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	deep := filepath.Join("shared", "a", "b")
	if err := mkdirPrivate(root, deep, "shared"); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		for _, rel := range []string{filepath.Join("shared", "a"), deep} {
			fi, err := os.Stat(filepath.Join(top, rel))
			if err != nil || fi.Mode().Perm() != 0o700 {
				t.Errorf("want a 0700 directory at %s, got %v, %v", rel, fi, err)
			}
		}
		fi, err := os.Stat(filepath.Join(top, "shared"))
		if err != nil || fi.Mode().Perm() != 0o755 {
			t.Errorf("the shared directory keeps its mode, got %v, %v", fi, err)
		}
	}
	if err := mkdirPrivate(root, deep, "shared"); err != nil {
		t.Errorf("an existing path should pass: %v", err)
	}
	if err := mkdirPrivate(root, filepath.Join("..", "elsewhere"), "shared"); err == nil {
		t.Error("a path outside the root should be refused")
	}
	file := filepath.Join(top, "shared", "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := mkdirPrivate(root, filepath.Join("shared", "file", "x"), "shared"); err == nil {
		t.Error("a regular file on the way should be refused")
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := os.Symlink(filepath.Join(top, "shared", "a"), filepath.Join(top, "shared", "link")); err != nil {
		t.Fatal(err)
	}
	if err := mkdirPrivate(root, filepath.Join("shared", "link", "x"), "shared"); err == nil {
		t.Error("a symlink on the way should be refused")
	}
}

func TestWrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proxy", "registry")
	reg := Registration{Project: "demo", ProjectDir: "/p", Files: []string{"nginx/sites/a.conf"}}
	if err := Write(dir, FileName("demo"), reg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, FileName("demo")))
	if err != nil || len(data) == 0 {
		t.Fatalf("registration not written: %v", err)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(filepath.Join(dir, FileName("demo"))); fi.Mode().Perm() != 0o644 {
			t.Errorf("mode %v, want 0644", fi.Mode().Perm())
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("a temp file was left behind: %d entries", len(entries))
	}
	// A path under a regular file cannot be created.
	if err := Write(filepath.Join(dir, FileName("demo"), "sub"), "x.json", reg); err == nil {
		t.Error("writing below a regular file should fail")
	}
}

func TestUnregisterAndLeaveUnknownProject(t *testing.T) {
	owner := t.TempDir()
	if removed, err := Unregister(owner, "ghost"); err != nil || removed != nil {
		t.Errorf("Unregister of an unknown project: %v, %v", removed, err)
	}
	if removed, err := Leave(context.Background(), owner, "ghost"); err != nil || removed != nil {
		t.Errorf("Leave of an unknown project: %v, %v", removed, err)
	}
}
