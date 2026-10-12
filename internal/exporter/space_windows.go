//go:build windows

package exporter

import "golang.org/x/sys/windows"

// Purpose: measure available host disk. Inputs: existing directory.
// Outputs: available bytes. Constraints: read-only.
func freeSpace(path string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var free uint64
	err = windows.GetDiskFreeSpaceEx(p, &free, nil, nil)
	return free, err
}
