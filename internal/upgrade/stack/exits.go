package stack

import (
	"errors"

	"github.com/nself-org/cli/internal/errs"
)

// WithExitClass binds the stack failure class ahead of any nested process exit.
// Inputs: structured stack error with its original cause. Outputs: explicit exit error.
// Constraints: E139 remains exit 2 and E403 remains exit 4 in both output modes.
func WithExitClass(err error) error {
	if err == nil {
		return nil
	}
	var ce *errs.CLIError
	if errors.As(err, &ce) {
		if entry, ok := errs.Registry[ce.Code]; ok && entry.Exit != 0 {
			return errs.ExitWith(entry.Exit, err)
		}
	}
	return errs.ExitWith(errs.ExitCodeFor(err), err)
}
