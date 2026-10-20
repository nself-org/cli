//go:build !windows

package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// execTrustProblem checks the file at resolved (symlinks already evaluated) and
// every directory from its parent up to and including resolvedRoot. Each must be
// owned by uid or root and carry no group or world write bit, so no other local
// user can plant or swap a binary the router is about to exec. It returns ""
// when the chain is trusted, else a short reason.
func execTrustProblem(resolved, resolvedRoot string, uid int) string {
	p := resolved
	for {
		info, err := os.Stat(p)
		if err != nil {
			return fmt.Sprintf("cannot inspect %s", p)
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Sprintf("cannot read the owner of %s", p)
		}
		if int(st.Uid) != uid && st.Uid != 0 {
			return fmt.Sprintf("%s is owned by uid %d", p, st.Uid)
		}
		if info.Mode().Perm()&0o022 != 0 {
			return fmt.Sprintf("%s is writable by group or others (%#o)", p, info.Mode().Perm())
		}
		if p == resolvedRoot {
			return ""
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "" // reached the filesystem root without meeting the plugins root
		}
		p = parent
	}
}
