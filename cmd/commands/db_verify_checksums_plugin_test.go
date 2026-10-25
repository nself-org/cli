package commands

// Purpose: `db verify-checksums --plugin` runs the same plugin-name grammar as
// `db migrate` (D-0602): a backup directory or a reserved name is refused
// before any project config or database is touched.

import (
	"context"
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

func TestDBVerifyChecksumsRefusesNonPluginNames(t *testing.T) {
	t.Chdir(t.TempDir()) // no project here: the guard must fire before the config load
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
	}
}
