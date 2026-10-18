// Package fslock is a small cross-process advisory file lock with a bounded wait.
// Purpose: serialise short read-modify-write sections that several nself processes share
// (the user's crontab, the entitlement token file) without hanging a caller forever.
// Inputs: a lock file path and a maximum wait. Outputs: an unlock func, or ErrTimeout.
// Constraints: the kernel drops the lock when the holder dies; the lock file is created 0600
// and its directory (0700) when absent; on platforms without flock the lock is a no-op.
// SPORT: advisory file lock for shared state.
package fslock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrTimeout means another process held the lock for the whole wait.
var ErrTimeout = errors.New("timed out waiting for a file lock")

// pollEvery is how often a waiting caller retries the non-blocking lock.
const pollEvery = 10 * time.Millisecond

// Acquire takes an exclusive lock on path, waiting at most wait, and returns the unlock func.
// The caller must call it exactly once (it is safe to defer).
func Acquire(path string, wait time.Duration) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		held, err := tryLock(f)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
		if !held {
			return func() { unlock(f); _ = f.Close() }, nil
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("%w: %s", ErrTimeout, path)
		}
		time.Sleep(pollEvery)
	}
}
