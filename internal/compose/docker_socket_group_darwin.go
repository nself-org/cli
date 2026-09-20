//go:build darwin

package compose

// socketGidAt (macOS): the host's /var/run/docker.sock is a symlink into the
// user's own Docker Desktop directory and stats as the user's group (staff,
// gid 20). That gid is NOT what the container sees. Docker Desktop runs the
// engine inside a VM and presents the socket to containers as root:root, so
// adding the host's gid 20 leaves the container denied — verified directly:
//
//	docker run -u 1000:1000 --group-add 20 ... docker version  -> DENIED
//	docker run -u 1000:1000 --group-add 0  ... docker version  -> OK
//
// So on macOS the correct group is the in-container owner, gid 0, not whatever
// the host reports. We still require the socket to exist before adding it, so
// a machine without Docker generates no group_add.

import "os"

func socketGidAt(path string) (string, bool) {
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	return "0", true
}
