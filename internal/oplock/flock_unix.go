//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package oplock

// Unix flock primitives. syscall.Flock is in the standard library, so the
// package adds no module dependency (no golang.org/x/sys promotion).
//
// The kernel releases a flock when the last descriptor of the open file
// description closes, which includes process death by SIGKILL: a crashed
// holder never leaves the lock held. Go opens files close-on-exec, so a child
// process never inherits the descriptor and never keeps the lock alive.

import (
	"errors"
	"os"
	"syscall"
)

// supported reports whether this platform can take the lock.
const supported = true

// tryLock takes an exclusive, non-blocking flock. held is true when another
// open file description owns it.
func tryLock(f *os.File) (held bool, err error) {
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		switch {
		case err == nil:
			return false, nil
		case errors.Is(err, syscall.EINTR):
			continue
		case errors.Is(err, syscall.EWOULDBLOCK):
			return true, nil
		}
		return false, err
	}
}

// unlock drops the flock explicitly; closing the file would also do it.
func unlock(f *os.File) {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
