package sharedproxy

// Tests for DetachHeld (D-0929): the owner lock taken by the detach stays held
// until the caller releases it, so an attach cannot reconnect the network
// between a fronted project's detach and its compose down.

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDetachHeldKeepsTheOwnerLock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")
	shortLockWait(t, 150*time.Millisecond)
	ctx := context.Background()

	release, err := DetachHeld(ctx, f.owner, b)
	if err != nil {
		t.Fatalf("DetachHeld: %v", err)
	}
	if release == nil {
		t.Fatal("DetachHeld returned a nil release")
	}

	// While held, a concurrent owner attach cannot get in.
	_, err = Attach(ctx, f.owner, "", []Registration{b})
	if err == nil || !strings.Contains(err.Error(), "another nself process") {
		release()
		t.Fatalf("attach during the held detach: want the lock-wait E506, got %v", err)
	}

	release()
	release() // idempotent

	// After the release the attach goes through.
	shortLockWait(t, 5*time.Second)
	if _, err := Attach(ctx, f.owner, "", []Registration{b}); err != nil {
		t.Fatalf("attach after release: %v", err)
	}
}
