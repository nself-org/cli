//go:build !unix

package portable

import "os"

// linkCount returns 1: hard-link counts are not read on this platform.
func linkCount(os.FileInfo) uint64 { return 1 }
