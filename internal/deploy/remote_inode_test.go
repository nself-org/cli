//go:build !windows

package deploy

import (
	"os"
	"syscall"
	"testing"
)

func statIno(t *testing.T, p string) uint64 {
	t.Helper()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return info.Sys().(*syscall.Stat_t).Ino
}
