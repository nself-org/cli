package imageswitch

// rollback.go — undo a switch: before the switch point (abort) and after it (--rollback).
//
// Purpose: any phase can be reversed. Before env is written, undo restarts the
// stack on the old volume, resets the read-only setting and removes the new
// volume (state this command created). After that point, rollback writes the
// previous env values back, regenerates compose and lock, starts the old
// volume, resets the read-only setting and starts the stack; the new volume is
// kept and named, because writes since the switch live only there.
// Inputs: the state. Outputs: PhaseRolledBack (state kept for the record) or no state.
// Constraints: the old volume is never removed. Every step is attempted and the
// failures are joined; the state is removed or marked rolled back only when all
// of them worked, so a half-undone switch stays visible to the guard.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

// undoTimeout bounds cleanup after an interrupt or a failure.
const undoTimeout = 10 * time.Minute

// undo reverses a switch that never changed env, compose or lock.
func (s *Switch) undo(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), undoTimeout)
	defer cancel()
	var failed []error
	step := func(what string, err error) bool {
		if err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", what, err))
			return false
		}
		return true
	}
	s.clearOneShots(ctx)
	// Postgres first: the other services start only on a healthy, writable old cluster.
	ready := true
	if info, err := s.d.Runtime.Inspect(ctx, s.cfg.Container); err != nil || !info.Running {
		ready = step("starting postgres on the old volume", s.d.Stack.Up(ctx, s.cfg.Service)) &&
			step("waiting for postgres", s.d.Stack.WaitHealthy(ctx, s.cfg.Service))
	}
	if s.st.ReadOnlySet && ready {
		ready = step("resetting the read-only setting", s.resetReadOnly(ctx))
	}
	step("removing the new volume", s.removeNewVolume(ctx))
	switch {
	case len(s.st.StoppedServices) == 0:
	case ready:
		step("restarting the stopped services", s.d.Stack.Up(ctx, s.st.StoppedServices...))
	default:
		step("restarting the stopped services", errors.New("not started: postgres is not ready on the old volume"))
	}
	if len(failed) > 0 {
		_ = s.persist("")
		return errors.Join(failed...)
	}
	_ = s.cleanStage()
	return removeState(s.cfg.ProjectDir)
}

// ErrNeedsConfirm is returned when a rollback after success was not confirmed.
func needsConfirm(newVolume string) error {
	return errs.New("E403", "rolling back after the switch needs confirmation").
		WithWhy("writes made since the switch exist only in the new volume " + newVolume + "; the old volume does not have them").
		WithFix("run again with --yes to go back to the old volume, which keeps the data as it was at the switch")
}

// Rollback reverses an incomplete or finished switch. After the switch point
// it needs yes.
func (s *Switch) Rollback(ctx context.Context, yes bool) (*Report, error) {
	st, err := LoadState(s.cfg.ProjectDir)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, fmt.Errorf("there is no image switch to roll back in this project")
	}
	s.st = st
	if st.Phase == PhaseRolledBack {
		return &Report{Status: "rolled-back", Phase: st.Phase, OldVolume: st.OldVolume, NewVolume: st.NewVolume, Warnings: []string{"already rolled back"}}, nil
	}
	switch st.Phase {
	case PhaseSwitching, PhaseSwitched, PhaseDone:
		if (st.Phase == PhaseDone || st.Phase == PhaseSwitched) && !yes {
			return nil, needsConfirm(st.NewVolume)
		}
		return s.rollbackAfter(ctx)
	}
	s.say("rolling back: nothing was switched yet")
	if err := s.undo(ctx); err != nil {
		return nil, coded("E512", "the rollback is incomplete", s.redact(err.Error(), 6), "")
	}
	return &Report{Status: "rolled-back", OldVolume: st.OldVolume, NewImage: st.NewImage}, nil
}

// rollbackAfter reverses a switch whose env, compose and lock may name the new volume.
func (s *Switch) rollbackAfter(ctx context.Context) (*Report, error) {
	var failed []error
	step := func(what string, err error) bool {
		if err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", what, err))
			return false
		}
		return true
	}
	if running, err := s.d.Stack.Running(ctx); step("listing services", err) && len(running) > 0 {
		step("stopping the stack", s.d.Stack.Stop(ctx, running...))
	}
	if step("restoring env, compose and lock", s.d.Applier.Apply(context.WithoutCancel(ctx), s.st.PrevEnv)) {
		// Postgres first; the other services start only on a healthy, writable old cluster.
		if step("starting postgres on the old volume", s.d.Stack.Up(ctx, s.cfg.Service)) &&
			step("waiting for postgres", s.d.Stack.WaitHealthy(ctx, s.cfg.Service)) &&
			step("resetting the read-only setting", s.resetReadOnly(ctx)) {
			step("starting the stack", s.d.Stack.Up(ctx, s.stackServices()...))
		} else {
			step("starting the stack", errors.New("not started: postgres is not ready on the old volume"))
		}
	}
	if len(failed) > 0 {
		_ = s.persist("")
		return nil, coded("E512", "the rollback is incomplete", s.redact(errors.Join(failed...).Error(), 6),
			"Fix the cause and run `nself db image switch --rollback --yes` again.")
	}
	_ = s.persist(PhaseRolledBack)
	_ = s.cleanStage()
	return &Report{
		Status: "rolled-back", Phase: PhaseRolledBack, OldVolume: s.st.OldVolume, NewVolume: s.st.NewVolume,
		Warnings:  []string{"writes made on the new volume since the switch are not in the old volume"},
		NextSteps: []string{"the new volume " + s.st.NewVolume + " was kept; remove it by hand when you no longer need it: docker volume rm " + s.st.NewVolume},
	}, nil
}

// withNote appends text to an error's cause, keeping its code.
func withNote(err error, note string) error {
	var ce *errs.CLIError
	if errors.As(err, &ce) {
		cp := *ce
		cp.Why = strings.TrimSpace(strings.TrimSuffix(cp.Why, ".") + ". " + note)
		return &cp
	}
	return fmt.Errorf("%w. %s", err, note)
}
