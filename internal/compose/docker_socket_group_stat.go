//go:build !windows && !darwin

package compose

// socketGidAt (Linux and other unixes): the Docker socket is bind-mounted into
// the container directly, so its ownership is preserved and the host's stat
// reports the same gid the container will see. Typically root:docker mode 660.

import (
	"os"
	"strconv"
	"syscall"
)

func socketGidAt(path string) (string, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", false
	}

	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		return "", false
	}

	return strconv.FormatUint(uint64(st.Gid), 10), true
}
