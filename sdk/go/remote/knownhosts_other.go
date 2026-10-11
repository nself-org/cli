//go:build !unix || aix || solaris || illumos

// Purpose: non-unix stand-ins for the pinned known_hosts lock and owner test.
// Inputs:  an open *os.File, or a FileInfo.
// Outputs: the lock is always granted; every file counts as root-owned.
// Constraints: matches lock_other.go: no flock and no meaningful owners here,
//              so only the in-process mutex (pinMu) orders writers.

package remote

import "os"

// tryLockFD always succeeds where flock(2) is unavailable.
func tryLockFD(_ *os.File, _ bool) (bool, error) { return true, nil }

// rootOwned is always true: owners are not meaningful here (see ownerOK).
func rootOwned(_ os.FileInfo) bool { return true }
