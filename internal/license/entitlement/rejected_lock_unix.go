//go:build unix

package entitlement

// Purpose: cross-process exclusion for the rejection file's read-modify-write.
// Inputs: an open lock file. Outputs: the file locked exclusively.
// Constraints: syscall.Flock only (no new module). The kernel drops the lock
// with the descriptor, process death included. The pattern is
// internal/invoke/audit_lock_unix.go.

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// rejectedLockWait bounds how long a writer waits for another process.
var rejectedLockWait = 10 * time.Second

func lockRejected(f *os.File) error {
	deadline := time.Now().Add(rejectedLockWait)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, syscall.EINTR):
			continue
		case !errors.Is(err, syscall.EWOULDBLOCK):
			return err
		case time.Now().After(deadline):
			return errors.New("the rejection record stayed locked by another process")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
