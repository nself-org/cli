package promote

// Purpose: Prevent an uncertain database restore from executing twice.
// Inputs: Durable recovery phase, backup identity, runner, and checkpoint writer.
// Outputs: Started and completed checkpoints or an operator recovery error.
// Constraints: Completion save failure retains the started phase in memory too.

import (
	"context"
	"fmt"
	"time"

	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"
	remotesdk "github.com/nself-org/cli/sdk/go/v2/remote"
)

func uncertainDBRestore(backup string) error {
	return errs.Newf("E489", "database restore of backup %s may already have run; needs operator", backup).
		WithFix("Verify the database on the backup host, then run deploy rollback <target> --db-restore-done to record completion; otherwise restore by hand.")
}

// CheckDBRestoreCompletion refuses recovery while a restore outcome is uncertain.
func CheckDBRestoreCompletion(rec *Record) error {
	if rec.RecoveryPhase == "db_restore_started" && rec.DBRestoredAt == "" {
		return uncertainDBRestore(rec.BackupID)
	}
	return nil
}

// RestoreDatabase checkpoints intent before running the remote restore.
func RestoreDatabase(ctx context.Context, r deploy.Remote, remotePath, backup string, phase, restored *string, checkpoint func() error) error {
	if *restored != "" {
		return nil
	}
	if *phase == "db_restore_started" {
		return uncertainDBRestore(backup)
	}
	*phase = "db_restore_started"
	if err := checkpoint(); err != nil {
		return fmt.Errorf("saving database restore intent: %w", err)
	}
	if _, err := r.Run(ctx, "cd "+remotesdk.ShellQuote(remotePath)+" && nself backup restore "+remotesdk.ShellQuote(backup)+" --yes"); err != nil {
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
