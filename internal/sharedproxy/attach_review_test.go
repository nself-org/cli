package sharedproxy

// Tests for the Codex review of P7-ADOPT-21 (2026-10-15): a cold owner, a
// foreign registered network, a recovery that must not disconnect a working
// project, symlinked and loose quarantine trees. They reuse the fake docker of
// attach_test.go.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func (f *proxyFixture) setState(st string) {
	f.t.Helper()
	f.write(filepath.Join(f.fake, "state"), st+"\n")
}

func (f *proxyFixture) state() string {
	b, _ := os.ReadFile(filepath.Join(f.fake, "state"))
	return strings.TrimSpace(string(b))
}

func TestAttachColdOwner(t *testing.T) {
	ctx := context.Background()
	startPoll, startWait = time.Millisecond, time.Second
	for _, st := range []string{"exited", "restarting"} {
		t.Run(st, func(t *testing.T) {
			f := newProxyFixture(t, "b_network")
			f.owner0()
			b := f.project("b")
			f.setState(st)

			// A fronted project's start leaves a cold owner alone.
			if _, err := Attach(ctx, f.owner, "b", []Registration{b}); !errors.Is(err, ErrOwnerNotRunning) || isE506(err) {
				t.Fatalf("a cold owner for a fronted start: %v", err)
			}
			if strings.Contains(f.calls(), "network connect") || strings.Contains(f.calls(), "start") {
				t.Fatalf("nothing may be connected or started:\n%s", f.calls())
			}

			// The owner's own start attaches first, then starts nginx.
			out, err := Attach(ctx, f.owner, "", []Registration{b})
			if err != nil || len(out.Attached) != 1 || f.state() != "running" || f.reloads() != 1 {
				t.Fatalf("cold owner: %+v %v state %s", out, err, f.state())
			}
			calls := f.calls()
			ci, si := strings.Index(calls, "network connect"), strings.Index(calls, "start -- ")
			if ci < 0 || si < ci {
				t.Errorf("the connect must come before the start:\n%s", calls)
			}
			if st == "restarting" && !strings.Contains(calls, "stop -- ") {
				t.Errorf("a restarting nginx must be stopped before the connect:\n%s", calls)
			}
		})
	}
}

func TestDetachStoppedOwner(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")
	if _, err := Attach(ctx, f.owner, "b", []Registration{b}); err != nil {
		t.Fatal(err)
	}
	f.setState("exited") // the container exists but is stopped; it still holds the network
	if err := Detach(ctx, f.owner, b); err != nil {
		t.Fatalf("a stopped owner container must be detached: %v", err)
	}
	if strings.Contains(f.nets(), "b_network") {
		t.Error("the stopped container must be disconnected from b_network")
	}
}

func TestQuarantineNeverDisconnectsAForeignNetwork(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network", "owner_network")
	f.owner0()
	b := f.project("b")
	c := f.project("c")
	c.Network = "owner_network" // a hand-edited registration
	if err := Write(DirOf(f.owner), FileName("c"), c); err != nil {
		t.Fatal(err)
	}
	f.write(filepath.Join(f.fake, "nets.txt"), "bridge\nowner_network\n")
	f.write(filepath.Join(f.fake, "broken.txt"), "nginx/sites/c.api.conf\n")

	if _, err := Attach(ctx, f.owner, "b", []Registration{b}); !isE506(err) {
		t.Fatalf("want E506, got %v", err)
	}
	if !strings.Contains(f.nets(), "owner_network") || strings.Contains(f.calls(), "network disconnect -- owner_network") {
		t.Errorf("the owner's own network must never be disconnected:\n%s", f.calls())
	}
}

func TestFailedRecoveryKeepsWorkingProjects(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network", "c_network")
	f.owner0()
	b := f.project("b")
	c := f.project("c")
	if _, err := Attach(ctx, f.owner, "c", []Registration{c}); err != nil {
		t.Fatal(err)
	}
	// C's new conf breaks nginx, and a second, unrelated file keeps nginx -t
	// failing after C is quarantined.
	f.write(filepath.Join(f.fake, "broken.txt"), "nginx/sites/c.api.conf\nnginx/sites/other.conf\n")
	f.write(filepath.Join(f.owner, "nginx", "sites", "other.conf"), "bogus\n")
	if _, err := Attach(ctx, f.owner, "b", []Registration{b}); !isE506(err) {
		t.Fatalf("want E506, got %v", err)
	}
	if !strings.Contains(f.nets(), "c_network") {
		t.Error("C was working before this call: its connection must survive a failed recovery")
	}
	if strings.Contains(f.nets(), "b_network") {
		t.Error("B was connected by this call: it must be undone")
	}
	if _, err := os.Stat(filepath.Join(f.owner, "nginx", "sites", "c.api.conf")); err != nil {
		t.Errorf("C's files must be put back: %v", err)
	}
}

func TestQuarantineModesAreEnforced(t *testing.T) {
	f := newProxyFixture(t)
	f.owner0()
	b := f.project("b")
	set, _ := Load(DirOf(f.owner), f.owner)
	// A loose pre-existing quarantine tree and a 0644 site file.
	qdir := filepath.Join(DirOf(f.owner), QuarantineDir, "b", "nginx", "sites")
	if err := os.MkdirAll(qdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(DirOf(f.owner), QuarantineDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Quarantine(f.owner, set, b); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{
		filepath.Join(DirOf(f.owner), QuarantineDir):      0o700,
		filepath.Join(DirOf(f.owner), QuarantineDir, "b"): 0o700,
		qdir:                              0o700,
		filepath.Join(qdir, "b.api.conf"): 0o600,
		filepath.Join(DirOf(f.owner), QuarantineDir, "b", "ssl", "certificates", "b-cert", "fullchain.pem"): 0o600,
	} {
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s mode = %v (%v), want %v", path, modeOf(fi), err, want)
		}
	}
	if back, err := Restore(f.owner, b); err != nil || len(back) == 0 {
		t.Fatalf("restore: %v %v", back, err)
	}
	for path, want := range map[string]os.FileMode{
		filepath.Join(f.owner, "nginx", "sites", "b.api.conf"):                   0o644,
		filepath.Join(f.owner, "ssl", "certificates", "b-cert", "fullchain.pem"): 0o644,
		filepath.Join(f.owner, "ssl", "certificates", "b-cert", "privkey.pem"):   0o600,
	} {
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != want {
			t.Errorf("restored %s mode = %v (%v), want %v", path, modeOf(fi), err, want)
		}
	}
}

func modeOf(fi os.FileInfo) os.FileMode {
	if fi == nil {
		return 0
	}
	return fi.Mode().Perm()
}

func TestRestoreRefusesASymlinkedQuarantineTree(t *testing.T) {
	f := newProxyFixture(t)
	f.owner0()
	b := f.project("b")
	outside := t.TempDir()
	f.write(filepath.Join(outside, "nginx", "sites", "b.api.conf"), "server { bogus }\n")
	qdir := filepath.Join(DirOf(f.owner), QuarantineDir)
	if err := os.MkdirAll(qdir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(qdir, "b")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	live := filepath.Join(f.owner, "nginx", "sites", "b.api.conf")
	if err := os.Remove(live); err != nil {
		t.Fatal(err)
	}
	back, err := Restore(f.owner, b)
	if err == nil || len(back) != 0 {
		t.Errorf("a symlinked quarantine tree must be refused: %v %v", back, err)
	}
	if _, err := os.Lstat(live); err == nil {
		t.Error("nothing may be moved into nginx/sites from outside the owner")
	}
	if _, err := os.Stat(filepath.Join(outside, "nginx", "sites", "b.api.conf")); err != nil {
		t.Errorf("the outside file must stay: %v", err)
	}
}
