//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package secrets

import (
	"os"
	"syscall"
	"time"
)

// lockRecipientStore serializes the complete read/audit/write transaction.
// A separate inode survives the atomic replacement of the ciphertext.
func lockRecipientStore(path string) (func(), error) {
	fd, err := syscall.Open(path+".lock", syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, decryptError()
	}
	f := os.NewFile(uintptr(fd), path+".lock")
	deadline := time.Now().Add(5 * time.Second)
	for {
		err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = f.Close() }, nil
		}
		if err != syscall.EINTR && err != syscall.EWOULDBLOCK || time.Now().After(deadline) {
			_ = f.Close()
			return nil, decryptError()
		}
		time.Sleep(10 * time.Millisecond)
	}
}
