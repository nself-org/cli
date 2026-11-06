package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/backup"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/controlplane"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

type promoteRollbackYesKey struct{}
type promoteRollbackDBDoneKey struct{}
type rollbackConfirmProjectKey struct{}

// Purpose: Carry rollback's explicit approval through the record-lock context.
// Inputs: Command flags and context. Outputs: Context with --yes approval.
// Constraints: No other deploy flag authorizes a production database restore.
func withPromoteRollbackConfirmation(ctx context.Context, cmd *cobra.Command) context.Context {
	yes, _ := cmd.Flags().GetBool("yes")
	// compat.V15(P7-DEPL-15): uncertain restore repeated -> explicit operator completion checkpoint
	if done, _ := cmd.Flags().GetBool("db-restore-done"); compat.V15() && done {
		ctx = context.WithValue(ctx, promoteRollbackDBDoneKey{}, true)
	}
	project, _ := cmd.Flags().GetString("confirm-project")
	ctx = context.WithValue(ctx, rollbackConfirmProjectKey{}, project)
	return context.WithValue(ctx, promoteRollbackYesKey{}, yes)
}

// rollbackProject is the name a manual rollback passes to `backup restore
// --confirm-project` (C2-01 review). typed is the operator's --confirm-project.
// A production database restore needs it: --yes skips only the yes/no prompt,
// and the name is never read from the local config there (the CLI matching its
// own config against the host is not consent). Elsewhere an empty flag falls
// back to the local PROJECT_NAME; an unreadable config fails a restore with
// its real error. Promote's automatic rollback does not come through here.
func rollbackProject(typed, workdir, target string, restore, prod bool) (string, error) {
	if typed = strings.TrimSpace(typed); typed != "" {
		return typed, nil
	}
	if restore && prod {
		return "", errs.Newf("E403", "deploy rollback restoring the production database of %s requires --confirm-project <project>; --yes alone is not enough and nothing was changed", target).
			WithFix(fmt.Sprintf("Run: nself deploy rollback %s --yes --confirm-project <project> (PROJECT_NAME on the target host)", target))
	}
	project, err := backup.RestoreProject(workdir)
	if err != nil && restore {
		return "", fmt.Errorf("deploy rollback: read PROJECT_NAME for the database restore: %w; nothing was changed", err)
	}
	return project, nil
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
