//go:build unix

package secrets

import (
	"os"
	"syscall"
)

// storeHasMultipleLinks detects ciphertext shared between store names.
// Inputs: Lstat metadata for a regular file.
// Outputs: whether the inode has more than one directory entry.
// Constraints: Unix stat metadata only. SPORT: D-0580.
func storeHasMultipleLinks(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink > 1
}
