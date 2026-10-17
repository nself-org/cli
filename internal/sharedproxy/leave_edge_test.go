package sharedproxy

// Tests for the failure exits of leave.go: a refused quarantine path, an
// unreadable registration, a stopped owner, and the nginx -t and reload checks
// after the project's files are gone.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveQuarantineOpenFailure(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nowhere")
	err := removeQuarantine(missing, "b", true)
	if !isE506(err) || !strings.Contains(err.Error(), "cannot open") {
		t.Fatalf("a missing owner dir must be an E506: %v", err)
	}
}

func TestLeaveRefusesASymlinkedQuarantine(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")
	outside := t.TempDir()
	f.write(filepath.Join(outside, "keep.txt"), "keep")
	qdir := filepath.Join(DirOf(f.owner), QuarantineDir)
	if err := os.MkdirAll(qdir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(qdir, "b")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	removed, err := Leave(ctx, f.owner, "b")
	if !isE506(err) || len(removed) != 0 {
		t.Fatalf("a symlinked quarantine must be refused: %v %v", removed, err)
	}
	if !f.live("b") {
		t.Error("a refused leave must leave the project's files alone")
	}
	if _, err := os.Stat(filepath.Join(DirOf(f.owner), FileName(b.Project))); err != nil {
		t.Errorf("the registration must stay: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "keep.txt")); err != nil {
		t.Errorf("nothing may be deleted through the symlink: %v", err)
	}
}

func TestLeaveUnreadableRegistrations(t *testing.T) {
	f := newProxyFixture(t)
	f.owner0()
	f.project("b")
	f.write(filepath.Join(DirOf(f.owner), "broken.json"), "{ not json")
	removed, err := Leave(context.Background(), f.owner, "b")
	if !isE506(err) || len(removed) != 0 || !f.live("b") {
		t.Fatalf("an unreadable set must stop the leave: %v %v live=%v", removed, err, f.live("b"))
	}
}

func TestLeaveWithAStoppedOwner(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	f.project("b")
	f.write(filepath.Join(f.fake, "stopped"), "") // no owner container at all
	removed, err := Leave(ctx, f.owner, "b")
	if err != nil || len(removed) == 0 {
		t.Fatalf("a missing owner container is not an error: %v %v", removed, err)
	}
	if f.live("b") || f.reloads() != 0 {
		t.Errorf("files must be gone and nothing reloaded (live=%v reloads=%d)", f.live("b"), f.reloads())
	}
}

func TestLeaveNginxChecksAfterRemoval(t *testing.T) {
	ctx := context.Background()
	t.Run("nginx -t fails", func(t *testing.T) {
		f := newProxyFixture(t, "b_network")
		f.owner0()
		f.project("b")
		f.write(filepath.Join(f.fake, "broken.txt"), "nginx/sites/other.conf\n")
		f.write(filepath.Join(f.owner, "nginx", "sites", "other.conf"), "bogus\n")
		removed, err := Leave(ctx, f.owner, "b")
		if !isE506(err) || !strings.Contains(err.Error(), "not reloaded") {
			t.Fatalf("want an E506 saying nothing was reloaded: %v", err)
		}
		if len(removed) == 0 || f.live("b") || f.reloads() != 0 {
			t.Errorf("the project's files are gone and no reload ran: %v live=%v reloads=%d", removed, f.live("b"), f.reloads())
		}
	})
	t.Run("reload fails", func(t *testing.T) {
		f := newProxyFixture(t, "b_network")
		f.owner0()
		f.project("b")
		variantDocker(t, reloadFails, `echo "reload boom" >&2; exit 1`)
		removed, err := Leave(ctx, f.owner, "b")
		if !isE506(err) || !strings.Contains(err.Error(), "reload") || len(removed) == 0 {
			t.Fatalf("want an E506 about the reload with the removed files listed: %v %v", removed, err)
		}
	})
	t.Run("disconnect fails but the files still go", func(t *testing.T) {
		f := newProxyFixture(t, "b_network")
		f.owner0()
		b := f.project("b")
		if _, err := Attach(ctx, f.owner, "b", []Registration{b}); err != nil {
			t.Fatal(err)
		}
		variantDocker(t, disconnect, "      disconnect) echo \"boom\" >&2; exit 1 ;;\n      unused) ")
		removed, err := Leave(ctx, f.owner, "b")
		if !isE506(err) || errors.Is(err, ErrOwnerNotRunning) || !strings.Contains(err.Error(), "b_network") {
			t.Fatalf("the detach failure must come back: %v", err)
		}
		if len(removed) == 0 || f.live("b") {
			t.Errorf("the files must be removed even so: %v live=%v", removed, f.live("b"))
		}
	})
}
