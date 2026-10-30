package build

import (
	"errors"
	"io/fs"
	"os"
)

// Purpose: match planned new-file modes independently of the process umask.
// Inputs: path, bytes and planned mode. Outputs: written file or error.
// Constraints: existing files retain their permissions and normal access checks.
// SPORT: build artifact publication.
func writeFilePlannedMode(path string, data []byte, perm fs.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if errors.Is(err, fs.ErrExist) {
		return os.WriteFile(path, data, perm)
	}
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Chmod(perm)
	}
	return errors.Join(err, f.Close())
}
