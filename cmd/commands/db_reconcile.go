package commands

// Purpose: "nself db reconcile [--apply]" — push repo-declared Hasura
// metadata drift (found by ScanMetadataDrift) to a live instance. Dry-run by
// default; --apply is required to actually write.
// Inputs: the cobra command/args. Outputs: the printed plan, and (with
// --apply) the applied result, or an error.
// Constraints: see internal/database/metadata_reconcile.go for the safety
// design (refuse-both-directions, targeted ops, one atomic bulk call).

import (
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/database"
	"github.com/spf13/cobra"
)

var dbReconcileCmd = &cobra.Command{
	Use:   "reconcile",
	Short: "Push repo-declared Hasura metadata (permissions, relationships, table tracking) to a live instance",
	Long: `Reconcile live Hasura metadata toward what hasura/metadata/** declares.

Dry-run by default: prints the plan and exits 0 without writing. Pass --apply
to push it. Never issues a full 'hasura metadata apply' (which replaces the
whole document and would untrack hasura-auth's own tables) — every change is
a targeted per-object call, sent as one atomic bulk operation.

Refuses unconditionally (no bypass) if the repo declares a table that does
not exist in Postgres, or if live tracks a table the repo does not declare.`,
	RunE: dataEnv(runDBReconcile),
}

func init() {
	dbReconcileCmd.Flags().Bool("apply", false, "Actually push the plan (default: dry-run)")
	dbReconcileCmd.Flags().String("env", "", "Project directory to read hasura/metadata/** from (default: current directory)")
	dbCmd.AddCommand(dbReconcileCmd)
}

func runDBReconcile(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	dir, _ := cmd.Flags().GetString("env")
	if dir == "" {
		dir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
	}
	apply, _ := cmd.Flags().GetBool("apply")

	plan, err := database.BuildReconcilePlan(cmd.Context(), cfg, dir)
	if err != nil {
		return fmt.Errorf("build reconcile plan: %w", err)
	}

	rec := dbReconcileData{Changes: []dbReconcileChange{}, Applied: apply && len(plan.Changes) > 0}
	for _, c := range plan.Changes {
		rec.Changes = append(rec.Changes, dbReconcileChange{Table: c.Table, Description: c.Description})
	}
	setData(cmd, rec)
	if len(plan.Changes) == 0 {
		fmt.Println("Nothing to reconcile: live metadata already matches the repo.")
		return nil
	}

	fmt.Printf("%d change(s):\n", len(plan.Changes))
	for _, c := range plan.Changes {
		fmt.Printf("  - %s: %s\n", c.Table, c.Description)
	}

	if !apply {
		fmt.Println("\nDry run. Re-run with --apply to push these to the live instance.")
		return nil
	}

	fmt.Printf("\nApplying %d change(s) as one transaction...\n", len(plan.BulkOps))
	if err := plan.Apply(cmd.Context(), cfg); err != nil {
		return fmt.Errorf("apply reconcile plan: %w", err)
	}
	fmt.Println("Reconcile complete.")
	return nil
}
