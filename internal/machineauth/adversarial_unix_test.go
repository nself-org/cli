//go:build unix

package machineauth

// Purpose: unix-only adversarial subtests (named pipes) for TestMachineAuthAdversarial.
// Inputs: a FIFO placed at the token path. Outputs: one subtest.
// Constraints: Lookup must refuse it without opening it (opening would block).

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

var unixSubtests = []struct {
	name string
	fn   func(*testing.T)
}{{"fifo-token-file", advFIFO}}

// advFIFO places a named pipe at the token path: Lookup must refuse it from
// Lstat alone, because opening a FIFO with no writer blocks forever.
func advFIFO(t *testing.T) {
	home := homeDir(t)
	dir := filepath.Join(home, ".nself")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "mcp-token"), 0o600); err != nil {
		t.Skip("mkfifo unavailable:", err)
	}
	_, _, err := Lookup()
	wantE426(t, err, "not a regular file")
}
