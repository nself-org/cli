//go:build !windows

package commands

// config_set_restore_unix.go — keepOwner for the config set restore
// (P7-LIVE-05): the restored env file keeps the owner it had, so a failed
// `sudo nself config set` does not leave a root-owned .env behind.

import (
	"os"
	"syscall"
)

// keepOwner gives path the uid/gid recorded in info. A nil info or one
// without unix ownership does nothing; an error (not permitted) makes the
// caller fall back to the in-place restore, which keeps the owner anyway.
func keepOwner(path string, info os.FileInfo) error {
	if info == nil {
		return nil
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if int(st.Uid) == os.Geteuid() && int(st.Gid) == os.Getegid() {
		return nil
	}
	return os.Chown(path, int(st.Uid), int(st.Gid))
}
