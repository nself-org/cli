//go:build unix && !aix && !solaris && !illumos

// Purpose: cross-process advisory locking for the pinned known_hosts file.
// Inputs:  an open *os.File (the sidecar "<Path>.lock") and shared/exclusive.
// Outputs: flock(2) held on that file until unlockFD or Close.
//              Also the owner check (uid is you or root) and O_NOFOLLOW.
// Constraints: flock is per open-file-description, so every caller opens its
//              own descriptor. Retries on EINTR. Production code locks with
//              tryLockFD (LOCK_NB, polled by waitLock with a deadline);
//              lockFD blocks until granted and is kept for tests.

package remote

import (
	"os"
	"syscall"
)

// openNoFollow makes the lock open fail on a symlink planted at <Path>.lock.
const openNoFollow = syscall.O_NOFOLLOW

// openNonBlock keeps open(2) on a FIFO at <Path>.lock from blocking; acquire
// then refuses it as not a regular file. flock itself still blocks.
const openNonBlock = syscall.O_NONBLOCK

// uidOK is the owner rule: the file belongs to the effective user or root.
func uidOK(uid, euid int) bool { return uid == euid || uid == 0 }

// ownerOK applies uidOK to fi's owner (true when the platform reports none).
func ownerOK(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}
	return uidOK(int(st.Uid), os.Geteuid())
}

// rootOwned reports uid 0 (true when the platform reports no owner).
func rootOwned(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}
	return st.Uid == 0
}

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

// lockFD takes LOCK_EX (exclusive) or LOCK_SH on f, retrying on EINTR. It
// blocks with no deadline: tests only; acquire uses waitLock.
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
