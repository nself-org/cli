//go:build unix && !aix && !solaris && !illumos

// Purpose: unix helpers for the pinned known_hosts checks and lock.
// Inputs:  an open *os.File (the sidecar "<Path>.lock"), or a FileInfo.
// Outputs: a non-blocking flock attempt; whether a file is owned by root.
// Constraints: flock is per open-file-description. Retries on EINTR.
//              EWOULDBLOCK (held elsewhere) is not an error: the caller polls.

package remote

import (
	"os"
	"syscall"
)

// tryLockFD tries LOCK_EX or LOCK_SH with LOCK_NB. It reports false, nil when
// another open file description holds a conflicting lock.
func tryLockFD(f *os.File, exclusive bool) (bool, error) {
	how := syscall.LOCK_SH | syscall.LOCK_NB
	if exclusive {
		how = syscall.LOCK_EX | syscall.LOCK_NB
	}
	for {
		err := syscall.Flock(int(f.Fd()), how)
		if err == nil {
			return true, nil
		}
		if err == syscall.EWOULDBLOCK {
			return false, nil
		}
		if err != syscall.EINTR {
			return false, err
		}
	}
}

// rootOwned reports uid 0 (true when the platform reports no owner).
func rootOwned(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}
	return st.Uid == 0
}
