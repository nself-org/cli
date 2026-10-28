package entitlement

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// openFDs counts this process's open descriptors via /dev/fd (darwin, linux).
func openFDs(t *testing.T) int {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("no /dev/fd on windows")
	}
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Skipf("cannot list /dev/fd: %v", err)
	}
	return len(entries)
}

// A failed OpenFile must surface its own error. Negating the guard at
// rejected.go:99:9 falls through to (*os.File)(nil).Close(), which reports
// os.ErrInvalid instead of the missing directory.
func TestWriteMarkerReturnsOpenFileErrorForMissingDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent", rejectedFile)
	err := writeMarker(missing, "gone")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v, want fs.ErrNotExist", err)
	}
	if errors.Is(err, os.ErrInvalid) {
		t.Fatalf("err = %v is os.ErrInvalid: the OpenFile error was replaced", err)
	}
}

// A successful writeMarker leaves the marker on disk and its descriptor closed.
// Negating the guard returns early on success and leaks every opened file.
func TestWriteMarkerClosesMarkerFile(t *testing.T) {
	dir := useRejectedDir(t)
	path := filepath.Join(dir, rejectedFile)
	if err := writeMarker(path, "warmup"); err != nil {
		t.Fatal(err)
	}
	const writes = 16
	before := openFDs(t)
	for i := 0; i < writes; i++ {
		if err := writeMarker(path, "closed"); err != nil {
			t.Fatal(err)
		}
	}
	if after := openFDs(t); after-before >= writes/2 {
		t.Fatalf("open descriptors grew from %d to %d over %d writes", before, after, writes)
	}
	if info, err := os.Stat(filepath.Join(dir, markerPrefix+"closed"+markerSuffix)); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("marker missing after write: %v", err)
	}
}
