//go:build !windows

package entitlement

import "os"

// syncDirectory persists the initial token's directory entry.
// Purpose: durable rename. Inputs: directory. Outputs: sync/close error.
// Constraints: file contents were synced separately. SPORT: entitlement durability.
func syncDirectory(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	return syncAndClose(f)
}

type directorySyncer interface {
	Sync() error
	Close() error
}

// syncAndClose closes the directory even when persisting its entry fails.
// Purpose: persist and release the directory. Inputs: handle. Outputs: error.
// Constraints: preserve sync errors before close errors. SPORT: entitlement durability.
func syncAndClose(f directorySyncer) error {
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
