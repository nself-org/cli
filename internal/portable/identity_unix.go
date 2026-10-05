//go:build unix

package portable

import (
	"os"
	"syscall"
)

// openFlags open a member for reading without following a final symlink and
// without blocking on a FIFO that replaced the file.
const openFlags = os.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK

// handleInfo returns the file identity and hard-link count from fi, the
// result of Stat on the open descriptor.
func handleInfo(_ *os.File, fi os.FileInfo) (fileID, uint64, error) {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return fileID{dev: uint64(st.Dev), ino: uint64(st.Ino)}, uint64(st.Nlink), nil
	}
	return fileID{}, 1, nil
}
