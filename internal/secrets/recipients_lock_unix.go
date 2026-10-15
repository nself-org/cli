//go:build !windows

package secrets

// Purpose: hold a cross-process recipient lock on Unix.
// Inputs: an open persistent sidecar file.
// Outputs: an exclusive flock, released when the descriptor closes.
// Constraints: retry interrupted waits; never lock the replaced ciphertext.
// SPORT: P7-TRUST-03, D-0579.

import (
	"errors"
	"os"
	"syscall"
)

func lockRecipientFile(f *os.File) error {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}
