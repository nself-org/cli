//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package secrets

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// Lock a stable sidecar: saving a store atomically replaces its inode.
func lockRecipientStore(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = f.Close() }, nil
		}
		if !errors.Is(err, syscall.EINTR) && !errors.Is(err, syscall.EWOULDBLOCK) || time.Now().After(deadline) {
			_ = f.Close()
			return nil, err
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// multiplyLinked refuses aliases that bypass the file-class consent guard.
func multiplyLinked(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return !ok || stat.Nlink > 1
}
