//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package invoke

// Purpose: the audit file lock on platforms without flock (Windows).
// Inputs: an open audit file. Outputs: nothing; the lock is a no-op.
// Constraints: the process-wide mutex (auditFileMu) still serialises every
// writer of one process. Windows runs the unix build under WSL2 (VMI); a second
// native process may interleave whole lines but never splits one, because each
// line is one append-mode write.

import (
	"os"
	"time"
)

// closeBeforeRotate is true: Windows cannot rename an open file.
const closeBeforeRotate = true

func auditLockDeadline() time.Time            { return time.Time{} }
func lockFileUntil(*os.File, time.Time) error { return nil }
