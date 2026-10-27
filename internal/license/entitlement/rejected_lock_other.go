//go:build !unix && !windows

package entitlement

import "os"

// Purpose: the rejection lock on platforms with neither flock nor LockFileEx.
// Constraints: the process-wide mutex still serialises one process's writers.
func lockRejected(*os.File) error { return nil }
