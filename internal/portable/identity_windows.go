//go:build windows

package portable

import (
	"os"
	"syscall"
)

// openFlags is used for manifest.json's path-based open. Member opens instead
// use relative native handles and refuse reparse points (member_windows.go).
const openFlags = os.O_RDONLY

// handleInfo returns the volume serial number and file index, and the hard-link
// count, read from the open handle with GetFileInformationByHandle.
func handleInfo(fh *os.File, _ os.FileInfo) (fileID, uint64, error) {
	var d syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(fh.Fd()), &d); err != nil {
		return fileID{}, 0, err
	}
	return fileID{
		dev: uint64(d.VolumeSerialNumber),
		ino: uint64(d.FileIndexHigh)<<32 | uint64(d.FileIndexLow),
	}, uint64(d.NumberOfLinks), nil
}
