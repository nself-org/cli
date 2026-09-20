//go:build windows

package compose

// socketGidAt (windows): Windows has no POSIX group ownership, and Docker
// Desktop exposes the engine over a named pipe rather than a unix socket, so
// there is no gid to add. Always report "no group", which makes the caller
// emit no group_add key at all.

func socketGidAt(_ string) (string, bool) {
	return "", false
}
