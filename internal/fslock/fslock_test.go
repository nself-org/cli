package fslock

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestAcquireExcludesAndReleases(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no flock")
	}
	path := filepath.Join(t.TempDir(), "sub", "x.lock")
	unlock, err := Acquire(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(path, 50*time.Millisecond); !errors.Is(err, ErrTimeout) {
		t.Fatalf("second Acquire while held = %v, want ErrTimeout", err)
	}
	unlock()
	again, err := Acquire(path, time.Second)
	if err != nil {
		t.Fatalf("Acquire after unlock: %v", err)
	}
	again()
}
