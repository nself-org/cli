package sharedproxy

// lock.go — one writer at a time per owner dir, and the locked public API.
//
// Purpose: review F3 of EPIC ADOPT D2 (P7-ADOPT-21). Attach, Detach, Leave,
// Unregister, Quarantine and Restore for one owner dir read and move the same
// files and networks; run in parallel (the owner's start beside a fronted
// project's start) they undo each other's work. Each public call holds an
// exclusive advisory lock on `<owner>/nginx/shared/.lock` for its whole run.
// Inputs: the owner dir.
// Outputs: Lock (a release func), and the locked entry points below; the
// unlocked bodies are the lower-case functions of the same names.
// Constraints: the lock file is opened inside an os.Root on the owner dir, mode
// 0600, and refused when nginx/shared or the file is a symlink. The wait is
// bounded by lockWait and ends in an E506 error. An owner dir with no
// nginx/shared has nothing to protect and takes no lock. The lock is not
// re-entrant: lower-case functions never call a locked one.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LockFile is the lock file name inside nginx/shared/.
const LockFile = ".lock"

// lockWait bounds how long a call waits for another one; lockPoll is the retry
// interval. Variables so tests do not wait a minute.
var (
	lockWait = 60 * time.Second
	lockPoll = 25 * time.Millisecond
)

// Lock takes the exclusive lock of ownerDir's shared proxy and returns its
// release function. A caller that writes into the owner outside this package
// (the fronted build) takes it too.
func Lock(ctx context.Context, ownerDir string) (func(), error) {
	noop := func() {}
	root, err := os.OpenRoot(ownerDir)
	if err != nil {
		return noop, e506("cannot open the shared proxy owner dir "+ownerDir, err, "")
	}
	defer func() { _ = root.Close() }()
	sharedRel, err := ownerRel(ownerDir, DirOf(ownerDir))
	if err != nil {
		return noop, e506("cannot lock the shared proxy", err, "")
	}
	if err := plainDirs(root, sharedRel); err != nil {
		return noop, e506("cannot lock the shared proxy", err, "")
	}
	if _, err := root.Lstat(sharedRel); os.IsNotExist(err) {
		return noop, nil
	}
	lockRel := filepath.Join(sharedRel, LockFile)
	if fi, err := root.Lstat(lockRel); err == nil && !fi.Mode().IsRegular() {
		return noop, e506("cannot lock the shared proxy", fmt.Errorf("%s is not a regular file", lockRel), "")
	}
	f, err := openLockFile(root, sharedRel, lockRel)
	if err != nil {
		return noop, e506("cannot open the shared proxy lock file", err, "")
	}
	key := filepath.Clean(ownerDir)
	deadline := time.Now().Add(lockWait)
	for {
		unlock, ok, err := tryLock(f, key)
		if err != nil {
			_ = f.Close()
			return noop, e506("cannot lock the shared proxy", err, "")
		}
		if ok {
			return func() { unlock(); _ = f.Close() }, nil
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return noop, e506(fmt.Sprintf("another nself process has been changing the shared proxy at %s for over %s", ownerDir, lockWait), nil, "")
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return noop, e506("waiting for the shared proxy lock was cancelled", ctx.Err(), "")
		case <-time.After(lockPoll):
		}
	}
}

// openLockFile opens (creating) the lock file. Two processes or goroutines
// creating it at once can make the loser's openat report ENOENT although the
// directory exists (seen under os.Root), so a not-exist result is retried while
// the directory is still there.
func openLockFile(root *os.Root, sharedRel, lockRel string) (*os.File, error) {
	var err error
	for range 20 {
		var f *os.File
		if f, err = root.OpenFile(lockRel, os.O_RDWR|os.O_CREATE, 0o600); err == nil {
			return f, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
		if _, serr := root.Lstat(sharedRel); serr != nil {
			return nil, err // the directory itself is gone
		}
		time.Sleep(time.Millisecond)
	}
	return nil, err
}

// locked runs fn under Lock(ownerDir).
func locked[T any](ctx context.Context, ownerDir string, fn func() (T, error)) (T, error) {
	release, err := Lock(ctx, ownerDir)
	if err != nil {
		var zero T
		return zero, err
	}
	defer release()
	return fn()
}

// Attach is attach under the owner lock.
func Attach(ctx context.Context, ownerDir, self string, regs []Registration) (Outcome, error) {
	return locked(ctx, ownerDir, func() (Outcome, error) { return attach(ctx, ownerDir, self, regs) })
}

// Detach is detach under the owner lock.
func Detach(ctx context.Context, ownerDir string, regs ...Registration) error {
	_, err := locked(ctx, ownerDir, func() (struct{}, error) { return struct{}{}, detach(ctx, ownerDir, regs...) })
	return err
}

// DetachHeld is Detach, but it keeps the owner lock after detaching and returns
// its release func (never nil). A caller about to remove the network (compose
// down) releases it afterwards, so a concurrent attach cannot reconnect the
// owner's nginx in between (D-0929). The release is returned on error too: the
// lock stays held until the caller is done either way.
func DetachHeld(ctx context.Context, ownerDir string, regs ...Registration) (release func(), err error) {
	release, err = Lock(ctx, ownerDir)
	if err != nil {
		return func() {}, err
	}
	return release, detach(ctx, ownerDir, regs...)
}

// Leave is leave under the owner lock.
func Leave(ctx context.Context, ownerDir, project string) ([]string, error) {
	return locked(ctx, ownerDir, func() ([]string, error) { return leave(ctx, ownerDir, project) })
}

// Unregister is unregister under the owner lock.
func Unregister(ownerDir, project string) ([]string, error) {
	return locked(context.Background(), ownerDir, func() ([]string, error) { return unregister(ownerDir, project) })
}

// Quarantine is quarantine under the owner lock.
func Quarantine(ownerDir string, set Set, reg Registration) ([]string, error) {
	return locked(context.Background(), ownerDir, func() ([]string, error) { return quarantine(ownerDir, set, reg) })
}

// Restore is restore under the owner lock.
func Restore(ownerDir string, reg Registration) ([]string, error) {
	return locked(context.Background(), ownerDir, func() ([]string, error) { return restore(ownerDir, reg) })
}
