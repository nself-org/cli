package promote

// Purpose: Prevent an uncertain database restore from executing twice.
// Inputs: Durable recovery phase, backup identity, runner, and checkpoint writer.
// Outputs: Started and completed checkpoints or an operator recovery error.
// Constraints: Completion save failure retains the started phase in memory too.
// The remote restore carries --confirm-project (C2-01): the ssh run has no
// stdin, and `backup restore` refuses bare --yes on production.

import (
	"context"
	"fmt"
	"time"

	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/restoreconsent"
	remotesdk "github.com/nself-org/cli/sdk/go/v2/remote"
)

func uncertainDBRestore(tag, backup string) error {
	return errs.Newf("E489", "database restore of backup %s (snapshot %s) may already have run; needs operator", backup, tag).
		WithFix("Verify the database on the backup host, then run deploy rollback <target> --db-restore-done to record completion; otherwise restore by hand.")
}

// CheckDBRestoreCompletion refuses recovery while a restore outcome is uncertain.
func CheckDBRestoreCompletion(rec *Record) error {
	if rec.RecoveryPhase == "db_restore_started" && rec.DBRestoredAt == "" {
		return uncertainDBRestore(rec.SnapshotTag, rec.BackupID)
	}
	return nil
}

// RestoreDatabase checkpoints intent before running the remote restore.
// project is the name the operator confirmed; it is passed as
// --confirm-project, and an empty one is refused before the checkpoint.
func RestoreDatabase(ctx context.Context, r deploy.Remote, remotePath, project, tag, backup string, phase, restored *string, checkpoint func() error) error {
	if *restored != "" {
		return nil
	}
	if *phase == "db_restore_started" {
		return uncertainDBRestore(tag, backup)
	}
	restoreCmd, err := restoreconsent.Command(backup, project)
	if err != nil {
		return err
	}
	*phase = "db_restore_started"
	if err := checkpoint(); err != nil {
		return fmt.Errorf("saving database restore intent: %w", err)
	}
	if _, err := r.Run(ctx, "cd "+remotesdk.ShellQuote(remotePath)+" && "+restoreCmd); err != nil {
		return fmt.Errorf("restoring promotion backup %s: %w", backup, err)
	}
	*restored, *phase = time.Now().UTC().Format(time.RFC3339Nano), "db_restored"
	if err := checkpoint(); err != nil {
		*restored, *phase = "", "db_restore_started"
		return fmt.Errorf("database restored but completion could not be saved: %w", err)
	}
	return nil
}

// MarkDBRestoreDone records the operator's explicit host verification.
func MarkDBRestoreDone(root string, rec *Record) error {
	if rec.RecoveryPhase != "db_restore_started" || rec.DBRestoredAt != "" {
		return errs.New("E401", "--db-restore-done requires an uncertain database restore checkpoint")
	}
	rec.DBRestoredAt, rec.RecoveryPhase = time.Now().UTC().Format(time.RFC3339Nano), "db_restored"
	save := SaveRecord
	if live, err := LoadRecord(root); err != nil {
		rec.DBRestoredAt, rec.RecoveryPhase = "", "db_restore_started"
		return err
	} else if live == nil || live.ID != rec.ID {
		attempt, err := LoadAttempt(root)
		if err != nil {
			rec.DBRestoredAt, rec.RecoveryPhase = "", "db_restore_started"
			return err
		}
		if attempt != nil && attempt.ID == rec.ID {
			save = SaveAttempt
		}
	}
	if err := save(root, *rec); err != nil {
		rec.DBRestoredAt, rec.RecoveryPhase = "", "db_restore_started"
		return err
	}
	return nil
}
