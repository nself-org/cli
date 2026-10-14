//go:build !windows

package entitlement

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSyncDirectoryErrors(t *testing.T) {
	if err := syncDirectory(filepath.Join(t.TempDir(), "missing")); !os.IsNotExist(err) {
		t.Fatalf("missing directory must preserve open error: %v", err)
	}
	if err := syncDirectory(os.DevNull); err == nil {
		t.Fatal("device fsync failure must be returned")
	}
}

type recordingDirectory struct {
	calls             []string
	syncErr, closeErr error
}

func (d *recordingDirectory) Sync() error {
	d.calls = append(d.calls, "sync")
	return d.syncErr
}

func (d *recordingDirectory) Close() error {
	d.calls = append(d.calls, "close")
	return d.closeErr
}

func TestSyncAndClosePreservesErrorsAndCloses(t *testing.T) {
	syncFailure := errors.New("directory sync failed")
	closeFailure := errors.New("directory close failed")
	for _, tc := range []struct {
		name      string
		directory recordingDirectory
		want      error
	}{
		{"success", recordingDirectory{}, nil},
		{"sync", recordingDirectory{syncErr: syncFailure}, syncFailure},
		{"close", recordingDirectory{closeErr: closeFailure}, closeFailure},
		{"both", recordingDirectory{syncErr: syncFailure, closeErr: closeFailure}, syncFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := syncAndClose(&tc.directory)
			if !errors.Is(err, tc.want) || !reflect.DeepEqual(tc.directory.calls, []string{"sync", "close"}) {
				t.Fatalf("got error=%v calls=%v; want error=%v and sync then close", err, tc.directory.calls, tc.want)
			}
		})
	}
}
