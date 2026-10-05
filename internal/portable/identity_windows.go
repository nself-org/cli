//go:build windows

package portable

import (
	"os"
	"syscall"
)

// openFlags: Windows has no O_NOFOLLOW; the Lstat of every component before
// the open and the identity check after it stand in.
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
