//go:build windows

package entitlement

import (
	"errors"
	"os"
	"syscall"
)

// syncDirectory attempts directory durability where Windows supports it.
// Purpose: durable rename. Inputs: directory. Outputs: supported sync/close error.
// Constraints: FlushFileBuffers cannot sync ordinary directory handles; the token
// contents are synced independently. SPORT: entitlement durability.
func syncDirectory(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = f.Sync()
	closeErr := f.Close()
	// Win32 directory flush errors: invalid function, access denied, invalid
	// handle, and not supported. Preserve every other I/O failure.
	if err != nil && !errors.Is(err, syscall.Errno(1)) && !errors.Is(err, syscall.Errno(5)) && !errors.Is(err, syscall.Errno(6)) && !errors.Is(err, syscall.Errno(50)) {
		return err
	}
	return closeErr
}
