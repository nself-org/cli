//go:build unix

package machineauth

// Purpose: the owner uid of a token file (unix build).
// Inputs: a file's FileInfo. Outputs: its uid.
// Constraints: stdlib syscall only; metadata read, nothing is written.

import (
	"os"
	"syscall"
)

// fileOwner and currentUID are variables so a test can stub the owner check.
var (
	fileOwner  = statOwner
	currentUID = os.Geteuid
)

// statOwner returns the owner uid recorded in fi, and false when fi carries none.
func statOwner(fi os.FileInfo) (int, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(st.Uid), true
}
