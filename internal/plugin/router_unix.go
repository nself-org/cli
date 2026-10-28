//go:build !windows

package plugin

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/nself-org/cli/internal/errs"
)

// verifyPluginPathOwnershipPlatform is the Unix implementation of
// verifyPluginPathOwnership. It walks every component from path up to root
// and refuses the chain when any component is owned by another uid or is
// writable by group/world outside the user's private group (umask 002).
func verifyPluginPathOwnershipPlatform(path, root string) error {
	uid := uint32(syscall.Getuid())
	cur := path
	for {
		info, err := os.Lstat(cur)
		if err != nil {
			return errs.Newf("E406", "plugin binary path component %q is not accessible", cur)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return errs.Newf("E406", "plugin binary path component %q has no stat info", cur)
		}
		if stat.Uid != uid {
			return errs.Newf("E406", "plugin binary path component %q is owned by uid %d, not %d", cur, stat.Uid, uid)
		}
		mode := info.Mode().Perm()
		if mode&0o002 != 0 {
			return errs.Newf("E406", "plugin binary path component %q is world-writable", cur)
		}
		if mode&0o020 != 0 {
			return errs.Newf("E406", "plugin binary path component %q is group-writable (run chmod g-w %q to fix)", cur, cur)
		}
		if cur == root || cur == "/" || cur == "." {
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return nil
}

