//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package catalog

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestWriteCacheWaitsForOtherProcessLock holds the flock through a separate
// descriptor, as another nself process would, and proves a cache write waits
// for it instead of interleaving (D-0875).
func TestWriteCacheWaitsForOtherProcessLock(t *testing.T) {
	dir := t.TempDir()
	f, err := os.OpenFile(filepath.Join(dir, lockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // test descriptor
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		writeCache(dir, sampleCatalog(1), `"e1"`)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("writeCache finished while another process held the cache lock")
	case <-time.After(150 * time.Millisecond):
	}
	if _, err := os.Stat(filepath.Join(dir, bodyFile)); err == nil {
		t.Fatal("body was written under another process's lock")
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("writeCache did not resume after the lock was released")
	}
	if c, _, ok := readCache(dir); !ok || c.etag != `"e1"` {
		t.Fatalf("cache after release: ok=%v etag=%q", ok, c.etag)
	}
}
