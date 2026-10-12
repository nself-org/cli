package stack

import (
	"context"
	"errors"
	"fmt"
	"maps"

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
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), recoveryTimeout())
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
	if fix := failureFix(o, j, cause); fix != "" {
		e = e.WithFix(fix)
	}
	return e
}

// rollback restores data and the exact previous image set, then proves row equality.
// Inputs: step-one backup and pre-update row counts. Outputs: restored row evidence.
// Constraints: no volume or backup deletion, and no recovery in production. Order:
// (1) every pinned previous image must exist, before anything is touched; (2) stop
// the new-image app services, postgres stays up; (3) restore; (4) start the pinned
// snapshot and remove orphans; (5) compare row counts.
func rollback(ctx context.Context, b Backend, j *Journal) error {
	if j.Prod || j.BackupID == "" || j.BeforeRows == nil {
		return fmt.Errorf("automatic rollback lacks non-production backup evidence")
	}
	if err := b.CheckPreviousImages(ctx, j); err != nil {
		return &imagesMissingError{cause: fmt.Errorf("previous images are not available, nothing was stopped or restored: %w", err)}
	}
	if err := b.StopServices(ctx, j); err != nil {
		return fmt.Errorf("stop new-image services before restore: %w", err)
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

// imagesMissingError marks a rollback that stopped before touching data or services.
// Inputs: the preflight cause. Outputs: an error that guidance can recognize.
// Constraints: unwraps to the cause so errors.Is and As keep working.
type imagesMissingError struct{ cause error }

// Error returns the preflight message.
func (e *imagesMissingError) Error() string { return e.cause.Error() }

// Unwrap exposes the docker or snapshot error.
func (e *imagesMissingError) Unwrap() error { return e.cause }
