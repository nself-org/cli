package commands

// Purpose: reject backup and reserved plugin names before config or database I/O.
// Inputs: invalid --plugin values. Outputs: E401. Constraints: no services required.

import (
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

func TestDBVerifyChecksumsRejectsInvalidPlugin(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, name := range []string{"notify.prev", "bin", "common"} {
		t.Run(name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetContext(t.Context())
			cmd.Flags().String("plugin", name, "")
			err := runDBVerifyChecksums(cmd, nil)
			if d := errs.Describe(err); d == nil || d.Code != "E401" {
				t.Fatalf("--plugin %q: got %v, want E401", name, err)
			}
		})
	}
}
