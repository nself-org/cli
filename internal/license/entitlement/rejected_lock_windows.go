//go:build windows

package entitlement

// Purpose: cross-process exclusion for the rejection file's read-modify-write.
// Inputs: an open lock file. Outputs: the file locked exclusively.
// Constraints: LockFileEx on the whole file, polled like the unix flock so the wait is
// bounded. Windows drops the lock when the handle closes, process death included.

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// rejectedLockWait bounds how long a writer waits for another process.
var rejectedLockWait = 10 * time.Second

func lockRejected(f *os.File) error {
	deadline := time.Now().Add(rejectedLockWait)
	for {
		ol := new(windows.Overlapped)
		err := windows.LockFileEx(windows.Handle(f.Fd()),
			windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
		switch {
		case err == nil:
			return nil
		case !errors.Is(err, windows.ERROR_LOCK_VIOLATION):
			return err
		case time.Now().After(deadline):
			return errors.New("the rejection record stayed locked by another process")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
