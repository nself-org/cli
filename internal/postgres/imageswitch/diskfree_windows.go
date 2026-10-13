//go:build windows

package imageswitch

import "errors"

// freeBytes is not available on Windows: the switch needs docker bind mounts of
// a POSIX stage directory and is a Linux and macOS command.
func freeBytes(string) (uint64, error) {
	return 0, errors.New("free disk space is not measured on Windows")
}
