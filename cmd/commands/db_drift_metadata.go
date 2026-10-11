package commands

// Purpose: RunE for "nself db drift --metadata" — Hasura metadata drift
// (permissions/relationships/table tracking), as opposed to the sibling
// "nself db drift scan/fix" which only checks np_* column conventions.
// Inputs: the cobra command/args. Outputs: printed findings; exits 1 (via
// returned error) when drift is found, matching the ask's literal command
// shape.
// Constraints: split into its own file (db_audit.go stays the flag/RunE
// wiring, db_drift.go stays the column-drift implementation).

import (
	"fmt"

	"github.com/nself-org/cli/internal/database"
	"github.com/spf13/cobra"
)

func runDBDriftMetadata(cmd *cobra.Command, _ []string) error {
	// --env is an environment name (or, for back-compat, a project directory):
	// resolve it before touching local config so a remote name never needs a
	// local project.
	dir, handled, err := resolveDBMetadataDir(cmd, "db", "drift", "--metadata")
	if handled {
		return err
	}
	if err != nil {
		return err
	}
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	findings, err := database.ScanMetadataDrift(cmd.Context(), cfg, dir)
	if err != nil {
		return fmt.Errorf("scan metadata drift: %w", err)
	}

	if len(findings) == 0 {
		fmt.Println("Hasura metadata matches the repo: no drift found.")
		return nil
	}

	fmt.Printf("Hasura metadata drift: %d finding(s)\n\n", len(findings))
	for _, f := range findings {
		if f.Role != "" {
			fmt.Printf("  %s [%s/%s]: %s\n", f.Table, f.Kind, f.Role, f.Detail)
		} else {
			fmt.Printf("  %s [%s]: %s\n", f.Table, f.Kind, f.Detail)
		}
	}
	fmt.Println("\nRun 'nself db reconcile' to preview a fix, or 'nself db reconcile --apply' to push it.")

	return fmt.Errorf("%d metadata drift finding(s)", len(findings))
}
