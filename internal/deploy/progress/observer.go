package progress

// Purpose: adapters from the deploy code to a Writer.
// Inputs: promote engine lifecycle calls; deployStep style (name, status)
// records.
// Outputs: progress events on the Writer.
// Constraints: no import of the promote package; the methods match the
// promote.Observer interface by shape and a compile-time check lives in the
// cmd wiring.

import (
	"time"
)

// PromoteObserver feeds the promote engine's synchronous lifecycle calls into a
// Writer. Its method set satisfies controlplane/promote.Observer. The events
// are labelled with the target environment and no server.
type PromoteObserver struct {
	w *Writer
}

// NewPromoteObserver returns an observer writing to w, labelling events with
// env (the promotion target). A nil w gives an observer that does nothing.
func NewPromoteObserver(w *Writer, env string) *PromoteObserver {
	return &PromoteObserver{w: w.At(env, "")}
}

// Start reports that step began.
func (o *PromoteObserver) Start(step string) { o.w.Event(step, PhaseStart, "") }

// Done reports that step finished. The engine's step time is not repeated:
// elapsed_ms already orders the events.
func (o *PromoteObserver) Done(step string, _ time.Duration) { o.w.Status(step, "done", "") }

// Failed reports that step failed with err.
func (o *PromoteObserver) Failed(step string, err error) {
	o.w.Status(step, "failed", errText(err))
}

// Skipped reports that step did not run, with the reason.
func (o *PromoteObserver) Skipped(step, reason string) { o.w.Status(step, "skipped", reason) }

// Rollback reports the compensating action of step: undone when err is nil,
// else the undo failed.
func (o *PromoteObserver) Rollback(step string, err error) {
	if err != nil {
		o.w.Event(step, PhaseRollback, "rollback failed: "+errText(err))
		return
	}
	o.w.Event(step, PhaseRollback, "undone")
}

// ResultForPromote maps a promote engine status ("ok", "failed", "rolled_back",
// "rollback_failed") to a data.result value; anything else is failed.
func ResultForPromote(status string) string {
	switch status {
	case ResultOK, ResultRolledBack, ResultRollbackFailed:
		return status
	}
	return ResultFailed
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
