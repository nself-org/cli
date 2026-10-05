package build

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/oplock"
)

// TestAcquireBuildLockHeld: a held project lock fails the build at once, in
// both compat modes, with an error naming the holder and nothing written: the
// behaviour of origin/main's O_EXCL build.lock (exit 1). It never waits and
// never builds unlocked.
func TestAcquireBuildLockHeld(t *testing.T) {
	dir := t.TempDir()
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
		start := time.Now()
		release, err := AcquireBuildLock(context.Background(), dir)
		if !errors.Is(err, oplock.ErrHeld) || release != nil || !strings.Contains(err.Error(), "another build is already running") || !strings.Contains(err.Error(), "nself config set") {
			t.Fatalf("a held lock must fail naming the holder, got release=%v err=%v", release != nil, err)
		}
		if time.Since(start) > 2*time.Second {
			t.Fatal("the build waited for the lock")
		}
		if _, err := Build(dir, BuildOptions{}); !errors.Is(err, oplock.ErrHeld) {
			t.Fatalf("Build under a held lock must fail with ErrHeld, got %v", err)
		}
		if _, err := os.Stat(dir + "/docker-compose.yml"); err == nil {
			t.Fatal("a build wrote under a held lock")
		}
	})
}

// TestAcquireBuildLockReentry: the holder's own nested Build re-enters through
// the exported token and the lock file is removed when the owner releases.
func TestAcquireBuildLockReentry(t *testing.T) {
	dir := t.TempDir()
	_ = os.Unsetenv(oplock.EnvToken)
	release, err := AcquireBuildLock(context.Background(), dir)
	if errors.Is(err, oplock.ErrUnsupported) {
		t.Skip("no flock on this platform")
	}
	if err != nil {
		t.Fatal(err)
	}
	inner, err := AcquireBuildLock(context.Background(), dir)
	if err != nil {
		t.Fatalf("a nested acquire must re-enter: %v", err)
	}
	inner()
	release()
	if _, err := os.Stat(oplock.LockPath(dir)); err == nil {
		t.Fatal("the owner left the lock file behind")
	}
	if os.Getenv(oplock.EnvToken) != "" {
		t.Fatal("the token leaked past the release")
	}
}
