//go:build unix && !aix && !solaris && !illumos

package remote

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestWaitLockPollDelays(t *testing.T) {
	for _, tc := range []struct {
		name   string
		wait   time.Duration
		delays []time.Duration
	}{
		// A deadline shorter than the first poll must still sleep 5ms.
		{"initial", time.Millisecond, []time.Duration{5 * time.Millisecond}},
		// Cross the cap twice to check that subsequent delays stay capped.
		{"cap", 550 * time.Millisecond, []time.Duration{5 * time.Millisecond, 10 * time.Millisecond, 20 * time.Millisecond, 40 * time.Millisecond, 80 * time.Millisecond, 160 * time.Millisecond, 200 * time.Millisecond, 200 * time.Millisecond}},
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
			now := time.Unix(0, 0)
			var delays []time.Duration
			err = waitLockWithClock(waiter, true, path, func() time.Time { return now }, func(delay time.Duration) {
				if len(delays) >= len(tc.delays) {
					t.Fatalf("too many polls: %v, next delay %s", delays, delay)
				}
				delays = append(delays, delay)
				now = now.Add(delay)
			})
			if err == nil {
				t.Fatal("held lock unexpectedly acquired")
			}
			if !reflect.DeepEqual(delays, tc.delays) {
				t.Fatalf("poll delays = %v, want %v", delays, tc.delays)
			}
		})
	}
}
