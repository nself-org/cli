//go:build unix

package licensing

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// openRegular must reject a FIFO or symlink at open time, so a swap after any
// earlier check cannot block a refresh or evade symlink rejection (D-1112).
func TestOpenRegularRejectsFIFOSymlinkAndAcceptsRegular(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "token")
	if err := os.WriteFile(regular, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(regular, link); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if f, err := openRegular(regular); err != nil {
			t.Errorf("regular rejected: %v", err)
		} else {
			_ = f.Close()
		}
		for _, p := range []string{fifo, link} {
			if f, err := openRegular(p); !errors.Is(err, ErrEntitlementExpired) {
				if f != nil {
					_ = f.Close()
				}
				t.Errorf("%s returned %v", filepath.Base(p), err)
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("openRegular blocked on a FIFO")
	}
}
