//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package catalog

// Purpose: the catalog cache lock on platforms without flock (Windows).
// Constraints: a no-op; the process-wide mutex in lockCache still serialises
// every goroutine of one process. Windows runs the unix build under WSL2 (VMI).

import "os"

// flockFile is a no-op without flock.
func flockFile(*os.File) error { return nil }
