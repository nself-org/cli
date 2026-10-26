package commands

// Purpose: `db verify-checksums --plugin` runs the same plugin-name grammar as
// `db migrate` (D-0602): a backup directory or a reserved name is refused
// before any project config or database is touched.

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

func TestDBVerifyChecksumsRefusesNonPluginNames(t *testing.T) {
	// The config loads fine from an empty dir and database.VerifyChecksums
	// re-checks the name (wrapped as "verify checksums: ..."), so E401 alone
	// does not prove the guard. Only the guard returns the error unwrapped,
	// before the config load and the database layer are reached.
	t.Chdir(t.TempDir())
	for _, bad := range []string{"notify.prev", "bin", "common", "status", "../x", "a/b"} {
		cmd := &cobra.Command{Use: "verify-checksums"}
		cmd.Flags().String("plugin", "", "")
		if err := cmd.Flags().Set("plugin", bad); err != nil {
			t.Fatal(err)
		}
		cmd.SetContext(context.Background())
		err := runDBVerifyChecksums(cmd, nil)
		if d := errs.Describe(err); d == nil || d.Code != "E401" {
			t.Errorf("--plugin %q: err = %v, want E401", bad, err)
		}
		if err != nil && strings.Contains(err.Error(), "verify checksums") {
			t.Errorf("--plugin %q: err = %v came from the database layer, want the guard's error", bad, err)
		}
	}
}
