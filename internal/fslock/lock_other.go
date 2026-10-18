//go:build !unix

package fslock

import "os"

// No flock here: callers keep their own atomicity (rename) and the lock is a no-op.
func tryLock(*os.File) (bool, error) { return false, nil }

func unlock(*os.File) {}
