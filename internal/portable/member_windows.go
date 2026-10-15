//go:build windows

package portable

import (
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// openMemberDirectory pins the resolved root, refusing a raced reparse point.
func openMemberDirectory(root string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	return checkedMemberHandle(h, root, true)
}

// openMemberAt opens exactly one validated component relative to a pinned
// directory. FILE_OPEN_REPARSE_POINT opens the link itself; the handle check
// rejects all reparse points, including directory junctions, before traversal.
func openMemberAt(parent *os.File, name string, directory bool) (*os.File, error) {
	n, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	oa := windows.OBJECT_ATTRIBUTES{
		RootDirectory: windows.Handle(parent.Fd()), ObjectName: n,
		Attributes: windows.OBJ_CASE_INSENSITIVE,
	}
	oa.Length = uint32(unsafe.Sizeof(oa))
	options := uint32(windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT | windows.FILE_OPEN_FOR_BACKUP_INTENT)
	if directory {
		options |= windows.FILE_DIRECTORY_FILE
	}
	var h windows.Handle
	err = windows.NtCreateFile(&h, windows.FILE_GENERIC_READ|windows.SYNCHRONIZE,
		&oa, &windows.IO_STATUS_BLOCK{}, nil, windows.FILE_ATTRIBUTE_NORMAL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN, options, 0, 0)
	if err != nil {
		var status windows.NTStatus
		if errors.As(err, &status) {
			err = status.Errno()
		}
		return nil, err
	}
	return checkedMemberHandle(h, name, directory)
}

// checkedMemberHandle transfers ownership only after inspecting the opened
// handle. Link and directory checks never depend on a path that can be swapped.
func checkedMemberHandle(h windows.Handle, name string, directory bool) (*os.File, error) {
	var info windows.ByHandleFileInformation
	err := windows.GetFileInformationByHandle(h, &info)
	if err == nil && (info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0)) {
		err = os.ErrPermission
	}
	if err != nil {
		_ = windows.CloseHandle(h)
		return nil, err
	}
	return os.NewFile(uintptr(h), name), nil
}
