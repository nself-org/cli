//go:build unix

package portable

import (
	"os"

	"golang.org/x/sys/unix"
)

// openMemberDirectory pins the resolved bundle root without following a link.
func openMemberDirectory(root string) (*os.File, error) {
	fd, err := unix.Open(root, openFlags|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), root), nil
}

// openMemberAt opens one component relative to a pinned parent, refusing
// symlinks atomically. O_NONBLOCK also prevents a raced FIFO from hanging.
func openMemberAt(parent *os.File, name string, directory bool) (*os.File, error) {
	flags := openFlags | unix.O_CLOEXEC
	if directory {
		flags |= unix.O_DIRECTORY
	}
	fd, err := unix.Openat(int(parent.Fd()), name, flags, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}
