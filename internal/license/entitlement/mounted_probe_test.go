package entitlement

import (
	"context"
	"errors"
	"testing"
	"time"
)

// The mount probe runs under a 5 s deadline: a zero or missing deadline would let a hung
// host inspection block a write, or cancel the probe before it starts.
func TestEntitlementMountedGivesTheProbeAFiveSecondDeadline(t *testing.T) {
	t.Cleanup(func() { SetMountProbe(nil) })
	var remaining time.Duration
	var hasDeadline bool
	SetMountProbe(func(ctx context.Context, _ string) (bool, error) {
		var dl time.Time
		dl, hasDeadline = ctx.Deadline()
		remaining = time.Until(dl)
		return false, nil
	})
	if entitlementMounted("/x") {
		t.Fatal("a probe answering not-mounted must report not mounted")
	}
	if !hasDeadline {
		t.Fatal("the probe context has no deadline")
	}
	if remaining < 3*time.Second || remaining > 5*time.Second {
		t.Fatalf("probe deadline in %v, want between 3s and 5s", remaining)
	}
}

// Without a probe, or when the probe fails, the inode is treated as mounted (preserved).
func TestEntitlementMountedPreservesWhenUnknown(t *testing.T) {
	t.Cleanup(func() { SetMountProbe(nil) })
	SetMountProbe(nil)
	if !entitlementMounted("/x") {
		t.Fatal("no probe must preserve the inode")
	}
	SetMountProbe(func(context.Context, string) (bool, error) { return false, errors.New("boom") })
	if !entitlementMounted("/x") {
		t.Fatal("a failing probe must preserve the inode")
	}
}
