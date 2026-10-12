//go:build !unix

package machineauth

// Purpose: the owner check on platforms without POSIX owners (Windows).
// Inputs: a file's FileInfo. Outputs: no owner; the check is a no-op.
// Constraints: checkTokenLocation already returns early where unixPerms is false.

import "os"

// fileOwner and currentUID are variables to match the unix build.
var (
	fileOwner  = func(os.FileInfo) (int, bool) { return 0, false }
	currentUID = func() int { return -1 }
)
