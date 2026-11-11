package remote

import (
	"fmt"
	"os"
	"time"
)

// waitLock polls a non-blocking flock until it is granted or pinLockWait has
// passed, so a hung holder cannot block every other nself process forever.
// Production code locks only through here; lockFD (blocking) is for tests.
func waitLock(f *os.File, write bool, lock string) error {
	return waitLockWithClock(f, write, lock, time.Now, time.Sleep)
}

// waitLockWithClock keeps backoff tests independent of scheduler delays.
func waitLockWithClock(f *os.File, write bool, lock string, now func() time.Time, sleep func(time.Duration)) error {
	deadline := now().Add(pinLockWait)
	delay := 5 * time.Millisecond
	for {
		ok, err := tryLockFD(f, write)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if !now().Before(deadline) {
			return fmt.Errorf("pinned host keys lock %s is held by another process; gave up after %s (find the holder with lsof)", lock, pinLockWait)
		}
		sleep(delay)
		delay = min(2*delay, 200*time.Millisecond)
	}
}
