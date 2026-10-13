package imageswitch

// switchover.go — the last step: stop postgres, write env, compose and lock, start (EPIC D6 step 7).
//
// Purpose: only after verify and the recheck passed does the project change:
// POSTGRES_IMAGE and POSTGRES_DATA_VOLUME are written and compose and the lock
// are regenerated through the build plan path (the Applier), then the stack starts.
// Inputs: the rechecked state. Outputs: PhaseSwitched, then PhaseDone.
// Constraints: the Applier undoes its env lines when it fails. A failure here
// leaves the state incomplete (E512 for other commands) with postgres stopped
// and the old volume untouched: --resume retries, --rollback goes back.

import (
	"context"
	"fmt"
	"time"
)

// newEnv is what the switch writes to the project env.
func (s *Switch) newEnv() map[string]string {
	return map[string]string{EnvImage: s.st.NewImage, EnvDataVolume: s.st.NewVolume}
}

// switchover stops postgres and rewrites env, compose and lock. A failure to stop
// postgres changed nothing and undoes the switch; a failure of the rewrite puts
// the previous env, compose and lock back and starts the old stack, as rollback does.
func (s *Switch) switchover(ctx context.Context) error {
	if err := s.persist(PhaseSwitching); err != nil {
		return err
	}
	// The new volume must still be the one this run restored into: a failed undo may have removed it.
	if err := s.checkNewVolume(ctx); err != nil {
		return s.abort(ctx, err)
	}
	s.say("stopping postgres on the old volume")
	if err := s.d.Stack.Stop(ctx, s.cfg.Service); err != nil {
		return s.abort(ctx, fmt.Errorf("stopping postgres: %w", err))
	}
	s.say("writing %s and %s, regenerating compose and the lock", EnvImage, EnvDataVolume)
	// An interrupt must not stop the rewrite half way: the Applier holds to the end.
	if err := s.d.Applier.Apply(context.WithoutCancel(ctx), s.newEnv()); err != nil {
		cause := fmt.Errorf("switching env, compose and lock: %w", err)
		if _, rerr := s.rollbackAfter(ctx); rerr != nil {
			return withNote(cause, "Putting the old stack back also failed: "+s.redact(rerr.Error(), 3))
		}
		return withNote(cause, "The previous env, compose and lock were put back and the old stack runs on the old volume.")
	}
	return s.persist(PhaseSwitched)
}

// stackServices are the services the stack ran when the switch began: postgres
// and the ones the switch stopped. A service the user had stopped stays stopped.
func (s *Switch) stackServices() []string {
	return append([]string{s.cfg.Service}, s.st.StoppedServices...)
}

// healthWait bounds the wait for postgres after the switch.
const healthWait = 3 * time.Minute

// start brings the whole stack up on the new volume and finishes.
func (s *Switch) start(ctx context.Context) error {
	s.say("starting the stack on %s", s.st.NewVolume)
	if err := s.d.Stack.Up(ctx, s.stackServices()...); err != nil {
		return fmt.Errorf("starting the stack: %w", err)
	}
	wctx, cancel := context.WithTimeout(ctx, healthWait)
	defer cancel()
	if err := s.d.Stack.WaitHealthy(wctx, s.cfg.Service); err != nil {
		return fmt.Errorf("postgres did not become healthy on the new volume: %w", err)
	}
	if err := s.onVolume(ctx, s.st.NewVolume); err != nil {
		return err
	}
	if err := s.persist(PhaseDone); err != nil {
		return err
	}
	return s.cleanStage()
}
