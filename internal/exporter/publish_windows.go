//go:build windows

package exporter

import (
	"os"

	"golang.org/x/sys/windows"
)

// Purpose: publish on Windows, where rename cannot replace an empty directory.
// Inputs: owned stage and output. Outputs: safe filesystem error.
// Constraints: directory-only removal refuses regular files and non-empty dirs;
// it never follows a junction into its target or recursively deletes contents.
func renameBundle(stage, out string) error {
	if fi, err := os.Lstat(out); err == nil {
		if !fi.IsDir() {
			return os.ErrExist
		}
		p, err := windows.UTF16PtrFromString(out)
		if err != nil {
			return err
		}
		if err = windows.RemoveDirectory(p); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(stage, out)
}
