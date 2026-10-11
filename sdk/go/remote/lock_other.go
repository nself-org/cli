//go:build !unix || aix || solaris || illumos

// Purpose: non-unix stand-in for the pinned known_hosts file lock.
// Inputs:  an open *os.File and shared/exclusive.
// Outputs: nothing; both calls succeed without locking.
// Constraints: documented no-op. Only the in-process mutex (pinMu) orders
//              writers on these platforms, so two processes writing the same
//              pin file at once can lose an update. Exists so the package
//              still compiles for windows CI, and on aix / solaris / illumos,
//              whose syscall package has no Flock.

package remote

import "os"

// openNoFollow is 0: no O_NOFOLLOW here.
const openNoFollow = 0

// openNonBlock is 0: no O_NONBLOCK here.
const openNonBlock = 0

// ownerOK is always true: owners are not meaningful here.
func ownerOK(_ os.FileInfo) bool { return true }

// rootOwned is always true: owners are not meaningful here (see ownerOK).
func rootOwned(_ os.FileInfo) bool { return true }

// tryLockFD always succeeds where flock(2) is unavailable.
func tryLockFD(_ *os.File, _ bool) (bool, error) { return true, nil }

// lockFD is a no-op where flock(2) is unavailable.
func lockFD(_ *os.File, _ bool) error { return nil }

// unlockFD is a no-op where flock(2) is unavailable.
func unlockFD(_ *os.File) {}
