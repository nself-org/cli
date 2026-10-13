//go:build !windows

package pintest

import (
	"os"
	"syscall"
)

// owner returns the owning uid of fi.
func owner(fi os.FileInfo) (int, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(st.Uid), true
}
