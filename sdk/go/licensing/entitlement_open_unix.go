//go:build unix

package licensing

import (
	"os"
	"syscall"
)

// Purpose: open a mounted token file without following a final symlink or
// blocking on a FIFO. Inputs: path. Outputs: open file or error.
// Constraints: O_NOFOLLOW and O_NONBLOCK make the swap race harmless; the
// caller still checks the opened descriptor is a regular file. SPORT: plugin.entitlement-token.
func openTokenFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}
