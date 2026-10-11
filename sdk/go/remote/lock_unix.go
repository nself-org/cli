//go:build unix

// Purpose: cross-process advisory locking for the pinned known_hosts file.
// Inputs:  an open *os.File (the sidecar "<Path>.lock") and shared/exclusive.
// Outputs: flock(2) held on that file until unlockFD or Close.
// Constraints: flock is per open-file-description, so every caller opens its
//              own descriptor. Retries on EINTR. Blocks until granted.

package remote

import (
	"os"
	"syscall"
)

// lockFD takes LOCK_EX (exclusive) or LOCK_SH on f, retrying on EINTR.
func lockFD(f *os.File, exclusive bool) error {
	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	for {
		err := syscall.Flock(int(f.Fd()), how)
		if err != syscall.EINTR {
			return err
		}
	}
}

// unlockFD releases the flock on f (closing f releases it too).
func unlockFD(f *os.File) {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
