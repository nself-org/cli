//go:build windows

package secrets

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// Lock a stable sidecar: saving a store atomically replaces its inode.
func lockRecipientStore(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	h := windows.Handle(f.Fd())
	deadline := time.Now().Add(5 * time.Second)
	for {
		ol := new(windows.Overlapped)
		err = windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
		if err == nil {
			return func() {
				_ = windows.UnlockFileEx(h, 0, 1, 0, new(windows.Overlapped))
				_ = f.Close()
			}, nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) || time.Now().After(deadline) {
			_ = f.Close()
			return nil, err
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// multiplyLinked reports no alias: a Windows file handle is needed to count
// links, and the guard here is advisory on that platform.
func multiplyLinked(os.FileInfo) bool { return false }
