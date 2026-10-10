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
)

// closeBeforeRotate is false: a unix rename works on an open, locked file, and
// renaming under the lock is what keeps a second rotator out.
const closeBeforeRotate = false

// lockFile takes a blocking exclusive flock on f.
func lockFile(f *os.File) error {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}
