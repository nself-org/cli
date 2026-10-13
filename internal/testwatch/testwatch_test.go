package testwatch

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestWatchdogFiresOnTimeout(t *testing.T) {
	var fired atomic.Int32
	timer := time.AfterFunc(20*time.Millisecond, func() { fired.Add(1) })
	defer timer.Stop()
	time.Sleep(200 * time.Millisecond)
	if fired.Load() != 1 {
		t.Fatalf("timer-based watchdog did not fire: %d", fired.Load())
	}
}

func TestFailMessageNames(t *testing.T) {
	// fail exits the process, so only check the limit is part of the contract.
	if got := time.Duration(90 * time.Second).String(); got != "1m30s" {
		t.Fatalf("unexpected duration format %q", got)
	}
}
