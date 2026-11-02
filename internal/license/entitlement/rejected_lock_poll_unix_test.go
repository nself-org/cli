//go:build unix

package entitlement

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func processCPU(t *testing.T) time.Duration {
	t.Helper()
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		t.Fatal(err)
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

// A contended lock is polled with a real sleep between attempts, not in a busy loop: waiting
// out the deadline must cost far less CPU than wall time, and must end in the stuck error.
func TestLockRejectedPollsWithASleepNotABusyLoop(t *testing.T) {
	dir := t.TempDir()
	holder, err := os.OpenFile(filepath.Join(dir, "lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	if err := lockRejected(holder); err != nil {
		t.Fatalf("first lock = %v", err)
	}
	waiter, err := os.OpenFile(filepath.Join(dir, "lock"), os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer waiter.Close()
	old := rejectedLockWait
	rejectedLockWait = 600 * time.Millisecond
	t.Cleanup(func() { rejectedLockWait = old })
	cpu, start := processCPU(t), time.Now()
	err = lockRejected(waiter)
	wall, used := time.Since(start), processCPU(t)-cpu
	if err == nil || !strings.Contains(err.Error(), "stayed locked") {
		t.Fatalf("contended lock = %v, want the stayed-locked error", err)
	}
	if wall < 600*time.Millisecond {
		t.Fatalf("gave up after %v, before the 600ms deadline", wall)
	}
	if used > 200*time.Millisecond {
		t.Fatalf("waiting burned %v of CPU in %v of wall time: the poll must sleep", used, wall)
	}
}
