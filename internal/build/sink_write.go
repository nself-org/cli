package build

import (
	"errors"
	"io/fs"
	"os"
)

// WriteFile writes bytes with the mode recorded by memSink.WriteFile.
// Inputs: a sink path, data and the mode for a newly created file.
// Outputs: a file whose mode is unaffected by the process umask, or an error.
// Constraints: existing files retain their permissions and must be writable.
func (d *diskSink) WriteFile(p string, data []byte, perm fs.FileMode) error {
	path := d.abs(p)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		return err
	}
	return os.Chmod(path, perm)
}
