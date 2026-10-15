package secrets

// Purpose: hold a cross-process recipient lock on Windows.
// Inputs: an open persistent sidecar file.
// Outputs: an exclusive byte-range lock, released when the handle closes.
// Constraints: block competing writers across the entire store transaction.
// SPORT: P7-TRUST-03, D-0579.

import (
	"os"

	"golang.org/x/sys/windows"
)

func lockRecipientFile(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &windows.Overlapped{})
}
