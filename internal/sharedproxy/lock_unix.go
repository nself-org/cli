//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package sharedproxy

// lock_unix.go — the advisory file lock behind Lock, on platforms with flock.
//
// The kernel drops a flock when the last descriptor closes, process death
// included, so a crashed holder never leaves the lock held. Go opens files
// close-on-exec, so a child process does not inherit the descriptor. Two
// goroutines of one process hold distinct descriptors and exclude each other.

import (
	"errors"
	"os"
	"syscall"
)

// tryLock takes an exclusive, non-blocking flock on f. ok is false when another
// open file description holds it. key is unused here.
func tryLock(f *os.File, _ string) (unlock func(), ok bool, err error) {
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		switch {
		case err == nil:
			return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }, true, nil
		case errors.Is(err, syscall.EINTR):
			continue
		case errors.Is(err, syscall.EWOULDBLOCK):
			return nil, false, nil
		}
		return nil, false, err
	}
}
