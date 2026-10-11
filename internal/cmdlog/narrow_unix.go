//go:build unix

package cmdlog

// Purpose: narrow files and directories the log wrote before D-0473 made them
// private (unix only).
// Constraints: only an entry owned by the effective uid is touched, and a mode
// is only ever narrowed, never widened. Errors are ignored: logging must never
// fail a command.

import (
	"os"
	"syscall"
)

// narrowTo clears group and other access on path when the effective user owns
// it and its mode is wider than max (a subset of 0o700).
func narrowTo(path string, max os.FileMode) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != os.Geteuid() {
		return
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		_ = os.Chmod(path, perm&max)
	}
}
