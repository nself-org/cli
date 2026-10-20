//go:build unix

package licensing

import (
	"os"
	"syscall"
)

// Reject symlink swaps and keep FIFO swaps from blocking before descriptor validation.
const tokenOpenFlags = os.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
