//go:build windows

package compose

// hostUser (windows): Windows has no POSIX uid/gid, and Docker Desktop's
// file sharing does not map ownership the way a Linux bind mount does.
// Report "unknown" so the caller keeps the documented default.

func hostUser() (string, bool) {
	return "", false
}
