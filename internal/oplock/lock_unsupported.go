//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package oplock

// Platforms without flock (Windows). The lock is a no-op there: Acquire
// returns ErrUnsupported and the guard runs the command unlocked with a
// notice. Windows is supported through WSL2 (VMI), where the unix build runs.

import "os"

// supported reports whether this platform can take the lock.
const supported = false

func tryLock(*os.File) (bool, error) { return false, ErrUnsupported }

func unlock(*os.File) {}
