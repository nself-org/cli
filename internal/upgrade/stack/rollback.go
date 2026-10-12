package stack

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

// failedStep records failure and invokes the bounded non-production recovery path.
// Inputs: failed operation and journal. Outputs: resumable or operator-only state.
// Constraints: production never restores automatically; backups remain retained.
func failedStep(ctx context.Context, o Options, b Backend, j *Journal, i int, cause error) error {
	j.Status = "failed"
	code := "E139"
	if o.Prod {
		j.Status = "awaiting-operator"
		if j.Steps[i].Name == "backup" {
			code = "E403"
		}
	} else if j.Steps[i].Name == "migrate" {
		j.Status = "rollback-running"
		if err := saveJournal(o.ProjectDir, j); err != nil {
			return errs.Wrap("E139", "Cannot record rollback before restoring", errors.Join(cause, err))
		}
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		if err := rollback(recovery, b, j); err != nil {
			cause = errors.Join(cause, err)
			j.Status = "rollback-failed"
		} else {
			j.Status = "rolled-back"
		}
	}
	if err := saveJournal(o.ProjectDir, j); err != nil {
		cause = errors.Join(cause, err)
	}
	e := errs.Wrap(code, fmt.Sprintf("Stack update step %s failed", j.Steps[i].Name), cause)
	if (o.Prod && verifiedBackupWithFailedStep(j)) || (j.BackupID != "" && j.Status == "rollback-failed") {
		e = e.WithFix(RestoreCommand(j.BackupID))
	}
	return e
}

// verifiedBackupWithFailedStep gates manual restore guidance on durable evidence.
// Inputs: journal. Outputs: whether a verified backup preceded a failed later step.
// Constraints: a backup identifier alone never authorizes restore guidance.
func verifiedBackupWithFailedStep(j *Journal) bool {
	if j.BackupID == "" || len(j.Steps) == 0 || j.Steps[0].Result != "ok" {
		return false
	}
	for _, step := range j.Steps[1:] {
		if step.Result == "failed" {
			return true
		}
	}
	return false
}

// rollback restores data and the exact previous image set, then proves row equality.
// Inputs: step-one backup and pre-update row counts. Outputs: restored row evidence.
// Constraints: no volume or backup deletion, and no recovery in production.
func rollback(ctx context.Context, b Backend, j *Journal) error {
	if j.Prod || j.BackupID == "" || j.BeforeRows == nil {
		return fmt.Errorf("automatic rollback lacks non-production backup evidence")
	}
	if err := b.Restore(ctx, j); err != nil {
		return fmt.Errorf("restore step-one backup: %w", err)
	}
	if err := b.RestartPrevious(ctx, j); err != nil {
		return fmt.Errorf("restart previous image set: %w", err)
	}
	rows, err := b.CountRows(ctx)
	if err != nil {
		return fmt.Errorf("count restored rows: %w", err)
	}
	j.AfterRows = rows
	if rows == nil || !maps.Equal(j.BeforeRows, rows) {
		return fmt.Errorf("restored per-table row counts differ from pre-update counts")
	}
	return nil
}

// RestoreCommand quotes an exact retained backup id for an operator shell command.
// Inputs: backup id. Outputs: executable restore guidance.
// Constraints: shell metacharacters in backup identifiers cannot execute commands.
func RestoreCommand(backupID string) string {
	return "nself backup restore '" + strings.ReplaceAll(backupID, "'", "'\"'\"'") + "'"
}
