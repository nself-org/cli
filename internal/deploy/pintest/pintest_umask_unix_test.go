//go:build !windows

package pintest

import (
	"os"
	"syscall"
	"testing"
)

// D-0697: under umask 002 t.TempDir() is 0775, which the pin store refuses as a
// group-writable ancestor of $HOME/.config/nself. Home must hand back 0700.
func TestHomeIsPrivateUnderGroupWritableUmask(t *testing.T) {
	old := syscall.Umask(0o002)
	defer syscall.Umask(old)
	home := Home(t)
	fi, err := os.Stat(home)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o700 {
		t.Fatalf("Home mode = %#o, want 0700", got)
	}
}
