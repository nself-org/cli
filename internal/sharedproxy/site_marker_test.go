package sharedproxy

// Tests for site_marker.go and leave.go: a registration cannot make the owner's
// own conf, or another project's, movable; a symlinked quarantine dir is never
// followed.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHandMadeRegistrationNeverMovesTheOwnersConf(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t)
	f.owner0()
	// The owner's own conf, and another project's, with and without a marker.
	f.write(filepath.Join(f.owner, "nginx", "sites", "default.conf"), "server {}\n")
	f.write(filepath.Join(f.owner, "nginx", "sites", "default.other.conf"), siteMarker("other")+"\nserver {}\n")
	x := New("default", filepath.Join(f.owner, "default"))
	x.Files = []string{"nginx/sites/default.conf", "nginx/sites/default.other.conf"}
	if err := Write(DirOf(f.owner), FileName("default"), x); err != nil {
		t.Fatal(err)
	}
	set, err := Load(DirOf(f.owner), f.owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range x.Files {
		if set.Removable(f.owner, "default", rel) {
			t.Errorf("%s must not be removable by project default", rel)
		}
	}
	if moved, err := Quarantine(f.owner, set, x); err != nil || len(moved) != 0 {
		t.Errorf("quarantine moved %v (%v)", moved, err)
	}
	if removed, err := Unregister(f.owner, "default"); err != nil || len(removed) != 0 {
		t.Errorf("unregister removed %v (%v)", removed, err)
	}
	// Unregister removed the registration: write it again for Leave.
	if err := Write(DirOf(f.owner), FileName("default"), x); err != nil {
		t.Fatal(err)
	}
	if _, err := Leave(ctx, f.owner, "default"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"default.conf", "default.other.conf"} {
		if _, err := os.Stat(filepath.Join(f.owner, "nginx", "sites", name)); err != nil {
			t.Errorf("%s must survive: %v", name, err)
		}
	}
}

func TestSiteOwnedBy(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		p := filepath.Join(root, "nginx", "sites", name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("b.api.conf", siteMarker("b")+"\nserver {}\n")
	write("b.late.conf", "# a\n# b\n# c\n"+siteMarker("b")+"\n")
	write("b.crlf.conf", siteMarker("b")+"\r\nserver {}\r\n")
	write("b.bare.conf", "server {}\n")
	write("b.other.conf", siteMarker("bb")+"\n")
	for rel, want := range map[string]bool{
		"nginx/sites/b.api.conf": true, "nginx/sites/b.crlf.conf": true,
		"nginx/sites/b.late.conf": false, "nginx/sites/b.bare.conf": false,
		"nginx/sites/b.other.conf": false, "nginx/sites/missing.conf": false,
		"nginx/sites/c.api.conf": false,
	} {
		if got := siteOwnedBy(root, rel, "b"); got != want {
			t.Errorf("siteOwnedBy(%s) = %v, want %v", rel, got, want)
		}
	}
}

func TestLeaveRefusesASymlinkedQuarantineDir(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	f.project("b")
	// <owner>/nginx/shared/quarantine -> <owner>/victim, which holds victim/b/keep.
	victim := filepath.Join(f.owner, "victim")
	f.write(filepath.Join(victim, "b", "keep.txt"), "keep\n")
	if err := os.Symlink(victim, filepath.Join(DirOf(f.owner), QuarantineDir)); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	removed, err := Leave(ctx, f.owner, "b")
	if !isE506(err) {
		t.Fatalf("want E506, got %v (removed %v)", err, removed)
	}
	if _, err := os.Stat(filepath.Join(victim, "b", "keep.txt")); err != nil {
		t.Errorf("a directory outside the quarantine was deleted through the symlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(DirOf(f.owner), FileName("b"))); err != nil {
		t.Errorf("a refused leave must leave the registration alone: %v", err)
	}
}
