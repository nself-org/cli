package commands

// Purpose: remove rollback-only dotenv keys using the planner's statement grammar.
// Inputs: the captured envLineWrite and key. Outputs: a private env file or error.
// Constraints: the caller holds interrupts and restores captured bytes on failure.

import (
	"os"

	"github.com/nself-org/cli/internal/reconcile"
	"github.com/nself-org/cli/internal/security"
)

func (w *envLineWrite) removeEnvLine(key string) error {
	if !w.existed {
		return nil
	}
	data, err := reconcile.EnvFileWithoutLine(w.prev, key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(w.path, data, 0o600); err != nil {
		return err
	}
	return security.EnforceFilePermissions(w.path, 0o600)
}
