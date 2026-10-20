//go:build unix

package secrets

import (
	"os"
	"syscall"
)

// storeHasMultipleLinks prevents a grant through another class's hardlink.
func storeHasMultipleLinks(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return !ok || stat.Nlink > 1
}
