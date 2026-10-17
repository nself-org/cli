package sharedproxy

// More error branches of attach_flow.go (the quarantine step) and detach.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttachQuarantineStepFailures(t *testing.T) {
	ctx := context.Background()
	t.Run("the quarantine tree is a symlink", func(t *testing.T) {
		f := newProxyFixture(t, "b_network")
		f.owner0()
		b := f.project("b")
		f.project("c")
		outside := t.TempDir()
		qdir := filepath.Join(DirOf(f.owner), QuarantineDir)
		if err := os.MkdirAll(qdir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(qdir, "c")); err != nil {
			t.Skip("symlinks unavailable:", err)
		}
		f.write(filepath.Join(f.fake, "broken.txt"), "nginx/sites/c.api.conf\n")
		out, err := Attach(ctx, f.owner, "b", []Registration{b})
		if !isE506(err) || !strings.Contains(err.Error(), "cannot quarantine") {
			t.Fatalf("want an E506 about the quarantine, got %v", err)
		}
		if len(out.Attached) != 0 || strings.Contains(f.nets(), "b_network") {
			t.Errorf("the attach must be undone: %+v\n%s", out, f.nets())
		}
		if entries, _ := os.ReadDir(outside); len(entries) != 0 {
			t.Errorf("nothing may be written through the symlink: %v", entries)
		}
		if !f.live("c") {
			t.Error("c's conf must stay where it was when it cannot be quarantined")
		}
	})
	t.Run("the reload after the quarantine fails", func(t *testing.T) {
		f := newProxyFixture(t, "b_network")
		f.owner0()
		b := f.project("b")
		f.project("c")
		f.write(filepath.Join(f.fake, "broken.txt"), "nginx/sites/c.api.conf\n")
		variantDocker(t, reloadFails, `echo "reload boom" >&2; exit 1`)
		out, err := Attach(ctx, f.owner, "b", []Registration{b})
		if !isE506(err) || !strings.Contains(err.Error(), "reload") {
			t.Fatalf("want an E506 about the reload, got %v", err)
		}
		if out.Reloaded || len(out.Attached) != 0 {
			t.Errorf("no success flag after a failed reload: %+v", out)
		}
	})
}

func TestDetachFailures(t *testing.T) {
	ctx := context.Background()
	t.Run("disconnect fails", func(t *testing.T) {
		f := newProxyFixture(t, "b_network")
		f.owner0()
		b := f.project("b")
		if _, err := Attach(ctx, f.owner, "b", []Registration{b}); err != nil {
			t.Fatal(err)
		}
		variantDocker(t, disconnect, "      disconnect) echo \"boom\" >&2; exit 1 ;;\n      unused) ")
		err := Detach(ctx, f.owner, b)
		if !isE506(err) || !strings.Contains(err.Error(), "b_network") {
			t.Fatalf("want an E506 naming the network, got %v", err)
		}
	})
	t.Run("owner container missing", func(t *testing.T) {
		f := newProxyFixture(t, "b_network")
		f.owner0()
		b := f.project("b")
		f.write(filepath.Join(f.fake, "stopped"), "")
		if err := Detach(ctx, f.owner, b); !errors.Is(err, ErrOwnerNotRunning) || isE506(err) {
			t.Errorf("a missing owner is the not-running sentinel: %v", err)
		}
	})
	t.Run("a foreign network is skipped", func(t *testing.T) {
		f := newProxyFixture(t, "owner_network")
		f.owner0()
		self := New("owner", filepath.Join(f.owner, "x"))
		f.write(filepath.Join(f.fake, "nets.txt"), "bridge\nowner_network\n")
		if err := Detach(ctx, f.owner, self); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(f.calls(), "network disconnect") || !strings.Contains(f.nets(), "owner_network") {
			t.Errorf("the owner's own network must never be disconnected:\n%s", f.calls())
		}
	})
}
