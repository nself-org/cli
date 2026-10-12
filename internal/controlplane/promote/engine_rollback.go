package promote

import (
	"context"
	"fmt"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

func (e Engine) rollback(ctx context.Context, st *State, result Result, failed int, cause error) (Result, error) {
	ctx = context.WithoutCancel(ctx)
	for i := failed; i >= 0 && e.Steps[i].Name != "compare"; i-- {
		step := e.Steps[i]
		if step.Name == "snapshot" && !st.MigrateStarted {
			e.skipped(step.Name, "migration not started")
			continue
		}
		if step.Undo == nil {
			continue
		}
		err := step.Undo(ctx, st)
		e.rollbackEvent(step.Name, err)
		if err != nil {
			return rollbackFailed(result, st, step.Name, err)
		}
		markRolledBack(&result, step.Name)
	}
	health := e.Steps[indexOf("health")]
	if health.Run != nil {
		if err := health.Run(ctx, st); err != nil {
			e.rollbackEvent("health", err)
			return rollbackFailed(result, st, "health", err)
		}
		e.rollbackEvent("health", nil)
	}
	result.Status = "rolled_back"
	return finishResult(result, st), errs.Wrap("E488", fmt.Sprintf("promote rolled back after %s failed", result.FailedStep), cause)
}

func rollbackFailed(result Result, st *State, step string, cause error) (Result, error) {
	result.Status = "rollback_failed"
	result = finishResult(result, st)
	what := fmt.Sprintf("promote rollback failed at %s for snapshot %s backup %s; needs operator", step, st.SnapshotTag, st.BackupID)
	return result, errs.Wrap("E489", what, cause).WithFix(fmt.Sprintf("Snapshot %s, backup %s: run nself backup restore %s --yes on the target, then run nself doctor.", st.SnapshotTag, st.BackupID, st.BackupID))
}

func resultFrom(st *State) Result {
	if st == nil {
		return Result{}
	}
	return finishResult(Result{Source: st.Source, Target: st.Target}, st)
}

func finishResult(result Result, st *State) Result {
	result.Source, result.Target = st.Source, st.Target
	result.Digests = append([]DigestRow(nil), st.Digests...)
	result.SnapshotTag, result.BackupID = st.SnapshotTag, st.BackupID
	return result
}

func markRolledBack(result *Result, name string) {
	for i := len(result.Steps) - 1; i >= 0; i-- {
		if result.Steps[i].Name == name {
			result.Steps[i].Status = "rolled_back"
			return
		}
	}
}

func indexOf(name string) int {
	for i, candidate := range StepNames {
		if candidate == name {
			return i
		}
	}
	return -1
}

func (e Engine) start(step string) {
	if e.Observer != nil {
		e.Observer.Start(step)
	}
}
func (e Engine) done(step string, d time.Duration) {
	if e.Observer != nil {
		e.Observer.Done(step, d)
	}
}
func (e Engine) failed(step string, err error) {
	if e.Observer != nil {
		e.Observer.Failed(step, err)
	}
}
func (e Engine) skipped(step, why string) {
	if e.Observer != nil {
		e.Observer.Skipped(step, why)
	}
}
func (e Engine) rollbackEvent(step string, err error) {
	if e.Observer != nil {
		e.Observer.Rollback(step, err)
	}
}
