package build

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/oplock"
)

// TestAcquireBuildLockHeld: with the project lock held by another holder, v1.5
// fails at once naming the holder, and v1.4 keeps the old behaviour (wait, then
// warn and build unlocked).
func TestAcquireBuildLockHeld(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(oplock.EnvToken, "")
	_ = os.Unsetenv(oplock.EnvToken)
	holder, err := oplock.Acquire(context.Background(), dir, oplock.Opts{Command: "nself config set"})
	if errors.Is(err, oplock.ErrUnsupported) {
		t.Skip("no flock on this platform")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()

	compattest.Both(t, func(t *testing.T) {
		var out bytes.Buffer
		oldErr, oldWait, oldClock := buildLockStderr, buildLockV14Wait, buildLockClock
		t.Cleanup(func() { buildLockStderr, buildLockV14Wait, buildLockClock = oldErr, oldWait, oldClock })
		buildLockStderr = &out
		now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
		buildLockClock = oplock.Clock{
			Now:   func() time.Time { return now },
			Sleep: func(_ context.Context, d time.Duration) error { now = now.Add(d); return nil },
		}
		release, err := AcquireBuildLock(context.Background(), dir)
		if os.Getenv("NSELF_V15") == "1" {
			if !errors.Is(err, oplock.ErrHeld) || !strings.Contains(err.Error(), "nself config set") || release != nil {
				t.Fatalf("v1.5 must fail naming the holder, got %v", err)
			}
			if out.Len() != 0 || now != time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) {
				t.Fatalf("v1.5 must not wait or print: %q", out.String())
			}
			return
		}
		if err != nil || release == nil {
			t.Fatalf("v1.4 must proceed unlocked, got %v", err)
		}
		release()
		if now.Sub(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)) < 30*time.Second {
			t.Fatal("v1.4 gave up before 30 s")
		}
		if s := out.String(); !strings.Contains(s, "waiting up to") || !strings.Contains(s, "building without the lock") {
			t.Fatalf("v1.4 notices missing: %q", s)
		}
	})
}
