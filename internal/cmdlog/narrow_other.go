//go:build !unix

package cmdlog

import "os"

// narrowTo is a no-op where POSIX modes and owners do not exist.
func narrowTo(string, os.FileMode) {}
