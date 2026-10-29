//go:build unix && !aix && !solaris && !illumos

package remote

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitLockPollDelays(t *testing.T) {
	for _, tc := range []struct {
		name          string
		wait, minimum time.Duration
	}{
		// A deadline shorter than the first poll must still sleep 5ms.
		{"initial", time.Millisecond, 5 * time.Millisecond},
		// 5+10+20+40+80+160+200ms. A zero cap busy-spins until 350ms.
		{"cap", 350 * time.Millisecond, 515 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "poll.lock")
			holder, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			defer holder.Close()
			if err := lockFD(holder, true); err != nil {
				t.Fatal(err)
			}
			defer unlockFD(holder)
			waiter, err := os.OpenFile(path, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer waiter.Close()
			old := pinLockWait
			pinLockWait = tc.wait
			defer func() { pinLockWait = old }()
			start := time.Now()
			err = waitLock(waiter, true, path)
			elapsed := time.Since(start)
			if err == nil {
				t.Fatal("held lock unexpectedly acquired")
			}
			if elapsed < tc.minimum {
				t.Fatalf("polling took %s, want at least %s", elapsed, tc.minimum)
			}
		})
	}
}
