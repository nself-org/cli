//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package invoke

// Purpose: the audit file lock on platforms without flock (Windows).
// Inputs: an open audit file. Outputs: nothing; the lock is a no-op.
// Constraints: the in-process mutex (Guard.auditMu) still serialises writers
// of one server process. Windows runs the unix build under WSL2 (VMI); a second
// native process may interleave whole lines but never splits one, because each
// line is one append-mode write.

import "os"

// closeBeforeRotate is true: Windows cannot rename an open file.
const closeBeforeRotate = true

// lockFile is a no-op without flock.
func lockFile(*os.File) error { return nil }
