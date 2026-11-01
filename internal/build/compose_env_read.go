package build

// Purpose: expose Compose's on-disk interpolation inputs for install preflight.
// Inputs: a project directory. Outputs: merged values, later files winning.
// Constraints: use config's parser and ignore shell-only project variables.

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/config"
)

// ReadComposeEnv reads the exact env-file set used by project Compose calls.
func ReadComposeEnv(root string) (map[string]string, error) {
	files := ComposeEnvFiles(root)
	if len(files) == 0 {
		path := filepath.Join(root, ".env")
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			return map[string]string{}, nil
		} else if err != nil {
			return nil, err
		}
		files = []string{path}
	}
	return config.ReadEnvFiles(files)
}
