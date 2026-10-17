package sharedproxy

import (
	"runtime"
	"testing"
)

// skipOnWindows skips tests that rely on Unix permission bits, directory modes
// or flock descriptor semantics (the Windows lock file only serialises goroutines).
func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("relies on Unix permission or descriptor semantics")
	}
}
