package build

// build_lock.go — the build's project lock (P7-LIVE-03). See AcquireBuildLock.

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/oplock"
)

// buildLockCommand is the command name recorded in the lock holder file.
const buildLockCommand = "nself build"

// AcquireBuildLock takes the project operation lock (internal/oplock), a flock
// on .nself/op.lock the kernel drops when the holder dies, so a SIGKILLed build
// leaves nothing stale. A descendant of the holder (NSELF_OPLOCK_TOKEN)
// re-enters it. A build that owns the lock removes the file while still holding
// it, as the old build.lock was removed, so a direct Build leaves the same tree.
// A held lock fails at once in every mode with an error naming the holder
// (errors.Is oplock.ErrHeld), exactly where origin/main's O_EXCL build.lock
// failed: no wait here (the command guard already waited in v1.4), nothing is
// written, and a build never runs unlocked. The caller must call release.
func AcquireBuildLock(ctx context.Context, workdir string) (release func(), err error) {
	l, err := oplock.Acquire(ctx, workdir, oplock.Opts{Command: buildLockCommand})
	switch {
	case errors.Is(err, oplock.ErrUnsupported):
		return func() {}, nil // no flock on this platform (Windows): as the guard does
	case err != nil:
		return nil, fmt.Errorf("another build is already running: %w", err)
	}
	if l.Reentrant() {
		return l.Release, nil
	}
	// Export the token so a nested Build (reconcile.Apply holds the lock across
	// its plan and its write) re-enters instead of finding itself held.
	prev, had := os.LookupEnv(oplock.EnvToken)
	_ = os.Setenv(oplock.EnvToken, l.Token())
	return func() {
		_ = os.Remove(oplock.LockPath(workdir))
		l.Release()
		if had {
			_ = os.Setenv(oplock.EnvToken, prev)
		} else {
			_ = os.Unsetenv(oplock.EnvToken)
		}
	}, nil
}
