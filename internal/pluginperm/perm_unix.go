//go:build !windows

package pluginperm

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Verify checks that every component of path up to and including root is
// safe for plugin execution: owned by the current uid and not writable by group
// or world, whatever the group (a shared group such as macOS staff would let
// other members replace plugin binaries). A missing or unstat-able component
// fails closed.
func Verify(path, root string) error {
	uid := uint32(syscall.Getuid())
	cur := path
	for {
		info, err := os.Lstat(cur)
		if err != nil {
			return fmt.Errorf("plugin binary path component %q is not accessible", cur)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("plugin binary path component %q has no stat info", cur)
		}
		if stat.Uid != uid {
			return fmt.Errorf("plugin binary path component %q is owned by uid %d, not %d", cur, stat.Uid, uid)
		}
		mode := info.Mode().Perm()
		if mode&0o002 != 0 {
			return fmt.Errorf("plugin binary path component %q is world-writable", cur)
		}
		if mode&0o020 != 0 {
			return fmt.Errorf("plugin binary path component %q is group-writable (run chmod g-w %q to fix)", cur, cur)
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
