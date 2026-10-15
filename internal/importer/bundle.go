package importer

import (
	"fmt"
	"path/filepath"

	"github.com/nself-org/cli/internal/portable"
)

// openBundle returns the verified staged reader or verifies sources that do not supply one.
// Purpose: avoid a redundant full read. Inputs: this extraction and the runner's stage.
// Outputs: a reader for that stage only. Constraints: never accept a source-directory reader.
func (x Extracted) openBundle(stage string) (*portable.Reader, error) {
	if x.VerifiedBundle == nil {
		return portable.Open(stage)
	}
	root, err := filepath.EvalSymlinks(stage)
	if err != nil || root != x.VerifiedBundle.Dir() {
		return nil, fmt.Errorf("verified bundle is not the staged copy")
	}
	return x.VerifiedBundle, nil
}
