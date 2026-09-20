//go:build !windows

package compose

// hostUser (unix): the uid:gid the CLI is running as.
//
// The admin container bind-mounts the project directory read-write. A
// container user that does not own that directory cannot write to it, so the
// admin's filesystem health check fails and /api/health answers 503.

import (
	"fmt"
	"os"
)

func hostUser() (string, bool) {
	uid, gid := os.Getuid(), os.Getgid()
	if uid < 0 || gid < 0 {
		return "", false
	}
	return fmt.Sprintf("%d:%d", uid, gid), true
}
