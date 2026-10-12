//go:build !windows

package exporter

import "golang.org/x/sys/unix"

// Purpose: measure available host disk. Inputs: existing directory.
// Outputs: bytes available to this uid. Constraints: no source mutation.
func freeSpace(path string) (uint64, error) {
	var s unix.Statfs_t
	if err := unix.Statfs(path, &s); err != nil {
		return 0, err
	}
	return s.Bavail * uint64(s.Bsize), nil
}
