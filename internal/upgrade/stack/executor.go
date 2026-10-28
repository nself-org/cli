package stack

import (
	"context"
	"fmt"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

// Execute runs or resumes the ordered stack plan after authorization.
// Inputs: caller-held operation lock, options and idempotent backend.
// Outputs: final journal and a coded failure. Constraints: dry runs write nothing.
func Execute(ctx context.Context, o Options, b Backend) (*Journal, error) {
	j, err := execute(ctx, o, b)
	return j, WithExitClass(err)
}

// execute performs the authorized state transitions behind the exit-class boundary.
// Inputs: options and backend. Outputs: durable journal and structured failure.
// Constraints: callers use Execute to preserve the stack error exit contract.
func execute(ctx context.Context, o Options, b Backend) (*Journal, error) {
	if o.DryRun {
		j, err := loadJournal(o.ProjectDir)
		if err != nil {
			return nil, errs.Wrap("E139", "Cannot preview stack update", err)
		}
		if j != nil {
			return j, nil
		}
		j = newJournal(o.Prod)
		j.Status = "planned"
		return j, nil
	}
	if !o.Yes && (o.Confirm == nil || !o.Confirm("Update stack and permit recovery from its verified backup?")) {
		return nil, errs.New("E403", "Stack update requires confirmation or --yes")
	}
	j, err := loadJournal(o.ProjectDir)
	if err != nil {
		return nil, errs.Wrap("E139", "Cannot resume stack update", err)
	}
	if j != nil && j.Status == "rolled-back" {
		if err := archiveJournal(o.ProjectDir, j); err != nil {
			return j, errs.Wrap("E139", "Cannot archive recovered stack update", err)
		}
		j = nil
	}
	if j == nil || j.Status == "complete" {
		j = newJournal(o.Prod)
	}
	if j.Prod != o.Prod {
		return j, errs.New("E403", "Stack update environment differs from its journal")
	}
	if j.Status == "rollback-running" || j.Status == "rollback-failed" {
		return j, errs.New("E139", "Stack update recovery requires operator reconciliation").WithFix(blockedFix(j))
	}
	if b == nil {
		return j, errs.New("E139", "Stack update backend is unavailable")
	}
	if firstPending(j) > 0 {
		if err := verifyProof(ctx, b, j); err != nil {
			if o.Prod {
				j.Status = "awaiting-operator"
				if saveErr := saveJournal(o.ProjectDir, j); saveErr != nil {
					err = fmt.Errorf("%w; persist production backup refusal: %v", err, saveErr)
				}
				// This run could not re-verify the dump: never offer it for restore.
				e := errs.Wrap("E403", "Stack update backup verification failed on resume", err)
				return j, e.WithFix(fmt.Sprintf("The step-one backup can no longer be trusted. Check it with: %s. Restore only a backup that verifies. To leave this state, either verify and restore by hand, or archive the journal by moving %s to %s.", VerifyCommand(j.BackupID), ".nself/state/update-stack.json", ".nself/state/update-stack."+j.RunID+".json"))
			}
			if !updateMutationAttempted(j) {
				err = resetBackupEvidence(j, err)
				// Later steps ran against the dump that was just discarded.
				for i := range j.Steps {
					j.Steps[i].Result = "pending"
				}
				if saveErr := saveJournal(o.ProjectDir, j); saveErr != nil {
					return j, errs.Wrap("E139", "Cannot clear invalid backup evidence", saveErr)
				}
				return j, backupError(false, err).WithFix("Invalid backup evidence was cleared before start or migrate ran. Retry nself update stack --yes to create and verify a fresh backup.")
			}
			return j, backupError(false, err).WithFix(fmt.Sprintf("The step-one backup can no longer be trusted. Check it with: %s. Restore only a backup that verifies. Verify and restore by hand, or archive the journal by moving %s to %s before starting a new update.", VerifyCommand(j.BackupID), ".nself/state/update-stack.json", ".nself/state/update-stack."+j.RunID+".json"))
		}
		if j.BeforeRows == nil || j.PreviousCompose == "" {
			return j, errs.New("E139", "Stack update journal lacks pre-update recovery evidence")
		}
	}
	j.Status = "running"
	if err := saveJournal(o.ProjectDir, j); err != nil {
		return j, errs.Wrap("E139", "Cannot persist stack update", err)
	}
	return executeSteps(ctx, o, b, j)
}

// executeSteps persists each transition before and after its operation.
// Inputs: authorized run and verified resume evidence. Outputs: updated journal.
// Constraints: a process killed during a step resumes that exact step.
func executeSteps(ctx context.Context, o Options, b Backend, j *Journal) (*Journal, error) {
	for i := firstPending(j); i < len(j.Steps); i++ {
		j.Steps[i].Result = "running"
		if err := saveJournal(o.ProjectDir, j); err != nil {
			return j, errs.Wrap("E139", "Cannot record running stack step", err)
		}
		started := time.Now()
		err := runStep(ctx, b, j, j.Steps[i].Name)
		j.Steps[i].DurationMS = time.Since(started).Milliseconds()
		if err != nil {
			j.Steps[i].Result = "failed"
			return j, failedStep(ctx, o, b, j, i, err)
		}
		j.Steps[i].Result = "ok"
		if err := saveJournal(o.ProjectDir, j); err != nil {
			return j, errs.Wrap("E139", "Cannot record successful stack step", err)
		}
	}
	j.Status = "complete"
	if err := saveJournal(o.ProjectDir, j); err != nil {
		return j, errs.Wrap("E139", "Cannot record completed stack update", err)
	}
	return j, nil
}

// firstPending finds the durable resume point.
// Inputs: validated journal. Outputs: first non-ok index, or total step count.
// Constraints: it never guesses completion from runtime state.
func firstPending(j *Journal) int {
	for i, s := range j.Steps {
		if s.Result != "ok" {
			return i
		}
	}
	return len(j.Steps)
}

// ResumeStep reports the operation a real invocation would attempt next.
// Inputs: validated journal. Outputs: next step name or empty for blocked recovery.
// Constraints: completed and recovered runs begin a fresh backup on the next run.
func ResumeStep(j *Journal) string {
	if j == nil || j.Status == "rollback-running" || j.Status == "rollback-failed" {
		return ""
	}
	if j.Status == "complete" || j.Status == "rolled-back" {
		return "backup"
	}
	if i := firstPending(j); i < len(j.Steps) {
		return j.Steps[i].Name
	}
	return ""
}

// runStep captures and verifies a backup before any image or database mutation.
// Inputs: ordered step. Outputs: completed operation evidence.
// Constraints: rollback row evidence must be collected before pull.
func runStep(ctx context.Context, b Backend, j *Journal, name string) error {
	if name != "backup" {
		return b.Run(ctx, name, j)
	}
	ctx, cancel := context.WithTimeout(ctx, stepTimeout("migrate"))
	defer cancel()
	if j.BackupID == "" {
		if err := b.Backup(ctx, j); err != nil {
			return resetBackupEvidence(j, fmt.Errorf("create update backup: %w", err))
		}
	}
	if err := verifyProof(ctx, b, j); err != nil {
		return resetBackupEvidence(j, err)
	}
	if j.PreviousCompose == "" {
		return resetBackupEvidence(j, fmt.Errorf("backup lacks previous image set evidence"))
	}
	if j.BeforeRows != nil {
		return nil
	}
	rows, err := b.CountRows(ctx)
	if err != nil {
		return fmt.Errorf("count pre-update rows: %w", err)
	}
	if rows == nil {
		return fmt.Errorf("pre-update row counts are missing")
	}
	j.BeforeRows = rows
	return nil
}

// resetBackupEvidence drops unusable step-one evidence so a retry takes a fresh dump.
// Inputs: journal and the failure. Outputs: the failure, tagged with the cleared backup id.
// Constraints: only journal fields change; the dump and snapshot files stay on disk.
func resetBackupEvidence(j *Journal, cause error) error {
	if j.BackupID == "" {
		return cause
	}
	id := j.BackupID
	j.BackupID, j.BackupSHA256 = "", ""
	j.BackupCreatedAt, j.BackupVerifiedAt = time.Time{}, time.Time{}
	j.PreviousCompose, j.PreviousComposeSHA256 = "", ""
	j.BeforeRows = nil
	return &evidenceReset{id: id, cause: cause}
}

// verifyProof re-verifies the retained backup in this invocation.
// Inputs: backup metadata and backend. Outputs: fresh verified evidence.
// Constraints: the backup must have been created no earlier than the run.
func verifyProof(ctx context.Context, b Backend, j *Journal) error {
	if j.BackupID == "" || j.BackupCreatedAt.Before(j.StartedAt) {
		return fmt.Errorf("backup was not created during this update run")
	}
	started := time.Now().UTC()
	if err := b.VerifyBackup(ctx, j); err != nil {
		return fmt.Errorf("verify update backup: %w", err)
	}
	if j.BackupVerifiedAt.Before(started) || j.BackupVerifiedAt.Before(j.BackupCreatedAt) {
		return fmt.Errorf("backup lacks verification evidence from this invocation")
	}
	return nil
}

// backupError classifies a failed production safety prerequisite.
// Inputs: production classification and underlying error. Outputs: coded error.
// Constraints: a production run fails E403 before the pull step.
func backupError(prod bool, err error) *errs.CLIError {
	code := "E139"
	if prod {
		code = "E403"
	}
	return errs.Wrap(code, "Stack update requires a backup created and verified in this run", err)
}

// Any attempted start or migration may have changed the stack, including a
// process interrupted while its durable result still says running.
func updateMutationAttempted(j *Journal) bool {
	for _, step := range j.Steps {
		if (step.Name == "start" || step.Name == "migrate") && step.Result != "pending" {
			return true
		}
	}
	return false
}
