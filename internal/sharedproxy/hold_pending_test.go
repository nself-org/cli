package sharedproxy

// Tests for D-0929 (DetachHolding keeps the owner lock) and D-0931 (a leave
// whose reload failed is retried; a failed attach starts a restarting owner
// back; the reload debt survives until paid).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDetachHoldingKeepsTheLockUntilReleased(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")
	if _, err := Attach(ctx, f.owner, "b", []Registration{b}); err != nil {
		t.Fatal(err)
	}
	oldWait, oldPoll := lockWait, lockPoll
	lockWait, lockPoll = 80*time.Millisecond, time.Millisecond
	t.Cleanup(func() { lockWait, lockPoll = oldWait, oldPoll })

	release, err := DetachHolding(ctx, f.owner, b)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(f.nets(), "b_network") {
		t.Error("DetachHolding must disconnect the network")
	}
	// An owner attach arriving between the detach and the compose down waits.
	if _, err := Attach(ctx, f.owner, "", []Registration{b}); !isE506(err) {
		t.Fatalf("an attach while the detach lock is held must time out, got %v", err)
	}
	if strings.Contains(f.nets(), "b_network") {
		t.Error("the owner reattached the network while the lock was held")
	}
	release()
	if _, err := Attach(ctx, f.owner, "", []Registration{b}); err != nil {
		t.Fatalf("after release the attach must work: %v", err)
	}
}

func TestLeaveReloadFailureIsRetried(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	f.project("b")
	marker := filepath.Join(DirOf(f.owner), ReloadPendingFile)

	variantDocker(t, reloadFails, `echo "reload boom" >&2; exit 1`)
	if _, err := Leave(ctx, f.owner, "b"); !isE506(err) {
		t.Fatalf("want the reload failure, got %v", err)
	}
	if _, err := os.Stat(marker); err != nil || !ReloadPending(f.owner) {
		t.Fatalf("a failed reload must leave the debt recorded: %v", err)
	}

	// The retry finds no registration but pays the reload, then forgets the debt.
	newProxyDockerOnPath(t, f)
	before := f.reloads()
	if _, err := Leave(ctx, f.owner, "b"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if f.reloads() != before+1 {
		t.Errorf("the retry must reload the owner once (reloads %d -> %d)", before, f.reloads())
	}
	if ReloadPending(f.owner) {
		t.Error("a paid reload must clear the marker")
	}
	// Nothing owed: a leave of an unknown project is a no-op again.
	if _, err := Leave(ctx, f.owner, "b"); err != nil || f.reloads() != before+1 {
		t.Errorf("no-op expected, got err %v reloads %d", err, f.reloads())
	}
}

func TestReloadIfPending(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	f.project("b")
	if ran, err := ReloadIfPending(ctx, f.owner); ran || err != nil || f.reloads() != 0 {
		t.Fatalf("nothing owed: ran=%v err=%v reloads=%d", ran, err, f.reloads())
	}
	variantDocker(t, reloadFails, `echo "reload boom" >&2; exit 1`)
	_, _ = Leave(ctx, f.owner, "b")
	if ran, err := ReloadIfPending(ctx, f.owner); !ran || !isE506(err) || !ReloadPending(f.owner) {
		t.Fatalf("a failing reload keeps the debt: ran=%v err=%v", ran, err)
	}
	newProxyDockerOnPath(t, f)
	if ran, err := ReloadIfPending(ctx, f.owner); !ran || err != nil || ReloadPending(f.owner) {
		t.Fatalf("paying the debt: ran=%v err=%v", ran, err)
	}
}

func TestLeaveWithAStoppedOwnerOwesNothing(t *testing.T) {
	f := newProxyFixture(t, "b_network")
	f.owner0()
	f.project("b")
	f.write(filepath.Join(f.fake, "stopped"), "")
	if _, err := Leave(context.Background(), f.owner, "b"); err != nil {
		t.Fatal(err)
	}
	if ReloadPending(f.owner) {
		t.Error("a stopped owner reads its files afresh on start: no reload is owed")
	}
}

func TestFailedAttachStartsARestartingOwnerBack(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")
	f.setState("restarting")
	failingConnectDocker(t, "b_network")
	if _, err := Attach(ctx, f.owner, "", []Registration{b}); !isE506(err) {
		t.Fatalf("want the connect failure, got %v", err)
	}
	if got := f.state(); got != "running" {
		t.Errorf("owner state after a failed attach = %q, want running", got)
	}
}

func TestFailedAttachLeavesAnExitedOwnerAlone(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")
	f.setState("exited")
	failingConnectDocker(t, "b_network")
	if _, err := Attach(ctx, f.owner, "", []Registration{b}); !isE506(err) {
		t.Fatalf("want the connect failure, got %v", err)
	}
	if got := f.state(); got != "exited" {
		t.Errorf("an owner this call did not stop must stay as found, state = %q", got)
	}
}

// newProxyDockerOnPath puts the plain fake docker back on PATH.
func newProxyDockerOnPath(t *testing.T, _ *proxyFixture) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(fakeProxyDocker), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}
