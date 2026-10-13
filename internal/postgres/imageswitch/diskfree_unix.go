//go:build !windows

package imageswitch

import "syscall"

// freeBytes returns the bytes available to an unprivileged user under path.
func freeBytes(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil //nolint:gosec // block counts are not negative
}
