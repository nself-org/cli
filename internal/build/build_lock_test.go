package build

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/oplock"
)

// holdLock takes the project lock as another command would.
func holdLock(t *testing.T, dir, command string) {
	t.Helper()
	_ = os.Unsetenv(oplock.EnvToken)
	h, err := oplock.Acquire(context.Background(), dir, oplock.Opts{Command: command})
	if errors.Is(err, oplock.ErrUnsupported) {
		t.Skip("no flock on this platform")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Release)
}

// TestAcquireBuildLockHeldByBuild: a lock held by a build refuses at once in
// both modes, naming the holder (origin/main's O_EXCL build.lock refused).
func TestAcquireBuildLockHeldByBuild(t *testing.T) {
	dir := t.TempDir()
	holdLock(t, dir, "nself build")
	compattest.Both(t, func(t *testing.T) {
		start := time.Now()
		release, err := AcquireBuildLock(context.Background(), dir)
		if !errors.Is(err, oplock.ErrHeld) || release != nil || !strings.Contains(err.Error(), "another build is already running") || !strings.Contains(err.Error(), "nself build (pid") {
			t.Fatalf("a held build lock must refuse naming the holder, got release=%v err=%v", release != nil, err)
		}
		if time.Since(start) > 2*time.Second {
			t.Fatal("the build waited for the lock")
		}
		if _, err := Build(dir, BuildOptions{}); !errors.Is(err, oplock.ErrHeld) {
			t.Fatalf("Build under a held lock must fail, got %v", err)
		}
		if _, err := os.Stat(dir + "/docker-compose.yml"); err == nil {
			t.Fatal("a build wrote under a held lock")
		}
	})
}

// TestAcquireBuildLockHeldByOtherCommand: v1.5 refuses naming the actual holder;
// v1.4 keeps origin/main's behaviour (the build goes ahead) but under the
// O_EXCL build.lock, so a second build is still refused, and never unlocked.
func TestAcquireBuildLockHeldByOtherCommand(t *testing.T) {
	dir := t.TempDir()
	holdLock(t, dir, "nself db migrate")
	compattest.Both(t, func(t *testing.T) {
		release, err := AcquireBuildLock(context.Background(), dir)
		if os.Getenv("NSELF_V15") == "1" {
			if !errors.Is(err, oplock.ErrHeld) || release != nil || !strings.Contains(err.Error(), "nself db migrate") || strings.Contains(err.Error(), "another build") {
				t.Fatalf("v1.5 must refuse naming the real holder, got %v", err)
			}
			return
		}
		if err != nil || release == nil {
			t.Fatalf("v1.4 must build past a non-build holder, got %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".nself", "build.lock")); err != nil {
			t.Fatal("v1.4 ran without the build.lock")
		}
		inner, err := AcquireBuildLock(context.Background(), dir)
		if err != nil {
			t.Fatalf("a nested acquire of the owner must re-enter: %v", err)
		}
		inner()
		release()
		if _, err := os.Stat(filepath.Join(dir, ".nself", "build.lock")); err == nil {
			t.Fatal("the lock file was left behind")
		}
	})
}

// TestAcquireBuildLockUnsupported: where flock is unsupported the O_EXCL file
// is the lock, as in origin/main: a second build is refused, release frees it,
// a nested acquire re-enters, and nothing runs unlocked.
func TestAcquireBuildLockUnsupported(t *testing.T) {
	old := acquireOplock
	acquireOplock = func(context.Context, string, oplock.Opts) (*oplock.Lock, error) { return nil, oplock.ErrUnsupported }
	t.Cleanup(func() { acquireOplock = old })
	dir := t.TempDir()
	release, err := AcquireBuildLock(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	inner, err := AcquireBuildLock(context.Background(), dir)
	if err != nil {
		t.Fatalf("the owner's nested acquire must re-enter: %v", err)
	}
	inner()
	// Another process: no token.
	tok := os.Getenv(envExclToken)
	_ = os.Unsetenv(envExclToken)
	if _, err := AcquireBuildLock(context.Background(), dir); err == nil || !strings.Contains(err.Error(), "another build is already running") {
		t.Fatalf("a second build must be refused, got %v", err)
	}
	_ = os.Setenv(envExclToken, tok)
	release()
	if os.Getenv(envExclToken) != "" {
		t.Fatal("the token leaked past the release")
	}
	again, err := AcquireBuildLock(context.Background(), dir)
	if err != nil {
		t.Fatalf("the lock must be free after release: %v", err)
	}
	again()
}

// TestAcquireBuildLockReentry: the holder's own nested Build re-enters through
// the exported token and the lock file is removed when the owner releases.
func TestAcquireBuildLockReentry(t *testing.T) {
	dir := t.TempDir()
	_ = os.Unsetenv(oplock.EnvToken)
	release, err := AcquireBuildLock(context.Background(), dir)
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
