//go:build !unix && !windows

package portable

import "os"

// openFlags: no O_NOFOLLOW here; component Lstat checks stand in.
const openFlags = os.O_RDONLY

// handleInfo: this platform exposes no file identity or link count; the
// snapshot falls back to size and mtime.
func handleInfo(*os.File, os.FileInfo) (fileID, uint64, error) { return fileID{}, 1, nil }
