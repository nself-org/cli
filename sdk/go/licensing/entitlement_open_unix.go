//go:build unix

package licensing

import (
	"os"
	"syscall"
)

// Purpose: open the entitlement file without following a symlink or blocking on a FIFO.
// Inputs: path. Outputs: open regular file or ErrEntitlementExpired.
// Constraints: no Lstat-then-open gap; the kind is checked on the opened descriptor.
// SPORT: plugin.entitlement-token.
func openRegular(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrEntitlementExpired
	}
	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, ErrEntitlementExpired
	}
	return f, nil
}
