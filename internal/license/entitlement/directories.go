package entitlement

import (
	"fmt"
	"os"
	"path/filepath"
)

// Purpose: refuse symlinked project state directories before creating files.
// Inputs: state path. Outputs: error. Constraints: inspect .nself and state only.
// SPORT: private entitlement writes.
func privateDirectories(state string) error {
	for _, dir := range []string{filepath.Dir(state), state} {
		info, err := os.Lstat(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("entitlement state directory must be a real directory: %s", dir)
		}
	}
	return nil
}
