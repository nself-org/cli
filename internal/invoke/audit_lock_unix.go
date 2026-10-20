//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package invoke

// Purpose: the cross-process flock of the machine audit file (unix build).
// Inputs: an open audit file. Outputs: the file locked exclusively.
// Constraints: syscall.Flock only (no new module). The kernel drops the lock
// when the last descriptor closes, process death included, and Go opens files
// close-on-exec, so a child never inherits it. The pattern is
// internal/oplock/flock_unix.go.

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// lockWait bounds how long an audit writer waits for another process's lock.
var lockWait = 5 * time.Second

// closeBeforeRotate is false: a unix rename works on an open, locked file, and
// renaming under the lock is what keeps a second rotator out.
const closeBeforeRotate = false

// auditLockDeadline includes time queued behind other in-process writers.
func auditLockDeadline() time.Time { return time.Now().Add(lockWait) }

// lockFileUntil takes a flock within the budget set before queuing.
func lockFileUntil(f *os.File, deadline time.Time) error {
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
			return errors.New("the machine audit file stayed locked by another process")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
