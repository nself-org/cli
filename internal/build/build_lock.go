package build

// build_lock.go — the build's project lock (P7-LIVE-03). See AcquireBuildLock.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/oplock"
)

// buildLockCommand is the command name recorded in the lock holder file.
const buildLockCommand = "nself build"

// exclLockFile is origin/main's lock file, used where flock is unsupported and,
// in v1.4, past a lock held by a command other than a build.
const exclLockFile = ".nself/build.lock"

// envExclToken carries the O_EXCL lock owner's token to a nested Build.
const envExclToken = "NSELF_BUILD_LOCK_TOKEN"

// acquireOplock is a seam so tests can force the unsupported-platform path.
var acquireOplock = oplock.Acquire

// AcquireBuildLock takes the project lock for a build; the caller must call
// release. A build never runs unlocked.
//
// Normally it takes the project operation lock (internal/oplock): a flock on
// .nself/op.lock the kernel drops when the holder dies, so a SIGKILLed build
// leaves nothing stale. A descendant of the holder (NSELF_OPLOCK_TOKEN)
// re-enters it. A build that owns the lock removes the file while still holding
// it, as the old build.lock was removed, so a direct Build leaves the same tree.
//
// Held by a build: refused at once (origin/main's O_EXCL build.lock refused),
// naming the holder. Held by another command: v1.5 refuses, naming it; v1.4
// keeps origin/main's behaviour (the command guard has already waited 30 s,
// then the build runs), still under origin/main's own O_EXCL
// .nself/build.lock, so two builds never overlap. Where flock is unsupported
// (Windows) the O_EXCL file is the lock, exactly as in origin/main.
func AcquireBuildLock(ctx context.Context, workdir string) (release func(), err error) {
	l, err := acquireOplock(ctx, workdir, oplock.Opts{Command: buildLockCommand})
	var held *oplock.HeldError
	switch {
	case err == nil:
	case errors.Is(err, oplock.ErrUnsupported):
		return acquireExcl(workdir)
	case errors.As(err, &held):
		// compat.V15(P7-LIVE-03): a lock held by another command does not stop a build (it takes the O_EXCL build.lock) -> a lock held by another command refuses the build
		if !compat.V15() && held.Known && held.Holder.Command != buildLockCommand {
			return acquireExcl(workdir)
		}
		if held.Known && held.Holder.Command == buildLockCommand {
			return nil, fmt.Errorf("another build is already running: %w", err)
		}
		return nil, fmt.Errorf("the project is locked by another command: %w", err)
	default:
		return nil, err
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
		restoreEnvVar(oplock.EnvToken, prev, had)
	}, nil
}

// acquireExcl takes the O_EXCL lock file. A nested Build of the owner re-enters
// through the token in the environment.
func acquireExcl(workdir string) (func(), error) {
	path := filepath.Join(workdir, exclLockFile)
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if tok := os.Getenv(envExclToken); tok != "" {
		if b, err := os.ReadFile(path); err == nil && string(b) == tok {
			return func() {}, nil
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("another build is already running (lock file exists: %s). If no other build is running, remove the lock file and retry", path)
		}
		return nil, fmt.Errorf("acquiring build lock: %w", err)
	}
	tok := strconv.Itoa(os.Getpid()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	_, _ = f.WriteString(tok)
	prev, had := os.LookupEnv(envExclToken)
	_ = os.Setenv(envExclToken, tok)
	return func() {
		_ = f.Close()
		_ = os.Remove(path)
		restoreEnvVar(envExclToken, prev, had)
	}, nil
}

// restoreEnvVar puts an environment variable back as it was.
func restoreEnvVar(key, val string, had bool) {
	if had {
		_ = os.Setenv(key, val)
	} else {
		_ = os.Unsetenv(key)
	}
}
