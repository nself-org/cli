//go:build unix

package licensing

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestTokenFileReplacementAtOpen(t *testing.T) {
	for _, kind := range []string{"symlink", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path, other := filepath.Join(dir, "token"), filepath.Join(dir, "other")
			for _, p := range []string{path, other} {
				if err := os.WriteFile(p, []byte("untrusted"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			data, err := readTokenFileWithOpen(path, func(p string, flags int, mode os.FileMode) (*os.File, error) {
				if err := os.Remove(p); err != nil {
					t.Fatal(err)
				}
				if kind == "symlink" {
					if err := os.Symlink(other, p); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := syscall.Mkfifo(p, 0600); err != nil {
						t.Fatal(err)
					}
					if flags&syscall.O_NONBLOCK == 0 {
						t.Fatal("FIFO replacement would block open")
					}
				}
				return os.OpenFile(p, flags, mode)
			})
			if err == nil || data != nil {
				t.Fatalf("replacement accepted: %q, %v", data, err)
			}
		})
	}
}
