// Package dirwritable tells, before anything is planned or deleted, whether the
// current user can unlink inside a directory tree (the traefik-config one-shot
// runs as root, so Docker creates its bind directories root-owned on Linux).
// It is read-only (access(2)) and identical in plan and apply mode.
package dirwritable

import (
	"io/fs"
	"path/filepath"
)

// FirstUnwritable finds the first directory that prevents unlinking, parent first.
func FirstUnwritable(parent, dir string) string {
	if !writable(parent) {
		return parent
	}
	var bad string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if bad != "" {
			return nil
		}
		if err != nil { // an unreadable directory cannot be emptied either
			bad = p
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if !writable(p) {
			bad = p
		}
		return nil
	})
	return bad
}
