//go:build windows

package ledger

// Purpose: Windows build stub. Windows is supported through WSL2 (VMI), where
// the unix build runs. A native Windows build has no OS lock: each write is
// still atomic (temp + rename), but concurrent writers can lose updates.

import (
	"os"
	"time"
)

func openLockFile(path string) (*os.File, error) {
	if st, err := os.Lstat(path); err == nil && st.Mode()&os.ModeSymlink != 0 {
		return nil, os.ErrInvalid
	}
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
}

func lockFile(_ *os.File, _ time.Duration) (func(), error) { return func() {}, nil }
