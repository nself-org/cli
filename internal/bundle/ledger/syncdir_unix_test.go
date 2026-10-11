//go:build !windows

package ledger

// syncDir tests (D-0374); unix only: the Windows syncDir is a no-op stub.

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestLedgerIgnoreUnsupportedSync: only "fsync on a directory is not supported"
// is dropped; a real I/O error comes back.
func TestLedgerIgnoreUnsupportedSync(t *testing.T) {
	cases := []struct {
		in   error
		want error
	}{
		{nil, nil},
		{syscall.EINVAL, nil},
		{syscall.ENOTSUP, nil},
		{&os.PathError{Op: "sync", Path: "d", Err: syscall.EINVAL}, nil},
		{syscall.EIO, syscall.EIO},
		{syscall.EBADF, syscall.EBADF},
	}
	for _, c := range cases {
		if got := ignoreUnsupportedSync(c.in); !errors.Is(got, c.want) || (c.want == nil && got != nil) {
			t.Errorf("ignoreUnsupportedSync(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestLedgerSyncDirMissing: a directory that cannot be opened is an error, not
// a silent success.
func TestLedgerSyncDirMissing(t *testing.T) {
	if err := syncDir(filepath.Join(t.TempDir(), "gone")); err == nil {
		t.Fatal("syncDir of a missing directory returned nil")
	}
	if err := syncDir(t.TempDir()); err != nil {
		t.Fatalf("syncDir of a real directory: %v", err)
	}
}
