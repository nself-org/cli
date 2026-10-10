//go:build !windows

package ledger

// Purpose: the OS file lock that serialises ledger writers on unix (flock).
// Constraints: syscall.Flock is standard library, so no module dependency. The
// kernel drops the lock when the descriptor closes, including on SIGKILL, so a
// crashed writer never leaves it held.

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// lockFile takes an exclusive flock on f, polling until wait elapses.
func lockFile(f *os.File, wait time.Duration) (func(), error) {
	deadline := time.Now().Add(wait)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("another nself process holds the bundle ledger lock (waited %s)", wait)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
