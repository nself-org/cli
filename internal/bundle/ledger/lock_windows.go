//go:build windows

package ledger

// Purpose: Windows build stub. Windows is supported through WSL2 (VMI), where
// the unix build runs; a native Windows build writes the ledger without an OS
// lock (still atomic via temp + rename).

import (
	"os"
	"time"
)

func lockFile(_ *os.File, _ time.Duration) (func(), error) { return func() {}, nil }
