//go:build !windows

package ledger

// Purpose: flush the ledger directory after the rename so the new entry is
// durable (D-0374).
// Constraints: open and fsync errors are returned, except EINVAL and ENOTSUP,
// which mean the filesystem cannot fsync a directory at all (nothing to flush).

import (
	"errors"
	"os"
	"syscall"
)

// syncDir fsyncs dir.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = ignoreUnsupportedSync(d.Sync())
	if cerr := d.Close(); err == nil {
		err = cerr
	}
	return err
}

// ignoreUnsupportedSync drops the errors a filesystem returns when it does not
// support fsync on a directory.
func ignoreUnsupportedSync(err error) error {
	if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) {
		return nil
	}
	return err
}
