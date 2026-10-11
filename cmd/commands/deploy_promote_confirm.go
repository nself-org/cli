package commands

import (
	"context"
	"fmt"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/controlplane"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

type promoteRollbackYesKey struct{}

// Purpose: Carry rollback's explicit approval through the record-lock context.
// Inputs: Command flags and context. Outputs: Context with --yes approval.
// Constraints: No other deploy flag authorizes a production database restore.
func withPromoteRollbackConfirmation(ctx context.Context, cmd *cobra.Command) context.Context {
	yes, _ := cmd.Flags().GetBool("yes")
	return context.WithValue(ctx, promoteRollbackYesKey{}, yes)
}

// Purpose: Confirm a production database restore before opening target remotes.
// Inputs: Current inventory and promotion record. Outputs: E403 or approval.
// Constraints: Restored databases and promotions without migration need no gate.
func confirmPromoteRecordRollback(ctx context.Context, inv *controlplane.Inventory, rec *cpromote.Record) error {
	// compat.V15(P7-DEPL-15): unconfirmed promote rollback -> require production database restore approval
	if !compat.V15() || !rec.MigrateStarted || rec.DBRestoredAt != "" || !prodClassFn(inv, rec.Target) {
		return nil
	}
	yes, _ := ctx.Value(promoteRollbackYesKey{}).(bool)
	if !yes && !deployConfirmPrompt(ctx, fmt.Sprintf("Restore database backup %s for production target %s", rec.BackupID, rec.Target)) {
		return errs.New("E403", "promotion rollback restoring a production-class database requires --yes or terminal confirmation")
	}
	return nil
}
