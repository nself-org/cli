//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package catalog

// Purpose: the cross-process flock of the catalog cache (unix build).
// Constraints: syscall.Flock only. The kernel drops the lock when the last
// descriptor closes, process death included. The pattern is
// internal/invoke/audit_lock_unix.go.

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// lockWait bounds how long a cache user waits for another process's lock: a
// holder stuck forever must not hang a plugin command.
var lockWait = 5 * time.Second

// flockFile takes an exclusive flock on f, waiting at most lockWait.
func flockFile(f *os.File) error {
	deadline := time.Now().Add(lockWait)
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
			return errors.New("the catalog cache stayed locked by another process")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
