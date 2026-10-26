//go:build !unix

package entitlement

import "os"

// Purpose: the rejection lock on platforms without flock (Windows).
// Constraints: the process-wide mutex still serialises one process's writers;
// Windows runs the unix build under WSL2 (VMI).
func lockRejected(*os.File) error { return nil }
