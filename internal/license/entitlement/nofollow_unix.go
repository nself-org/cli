//go:build unix

package entitlement

import (
	"os"
	"syscall"
)

// The Unix open flag rejects symlinks before acquiring the writable handle.
const noFollowFlag = syscall.O_NOFOLLOW

func singleLink(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1
}
