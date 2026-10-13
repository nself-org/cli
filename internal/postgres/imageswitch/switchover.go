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

// switchover stops postgres and rewrites env, compose and lock.
func (s *Switch) switchover(ctx context.Context) error {
	if err := s.persist(PhaseSwitching); err != nil {
		return err
	}
	s.say("stopping postgres on the old volume")
	if err := s.d.Stack.Stop(ctx, s.cfg.Service); err != nil {
		return fmt.Errorf("stopping postgres: %w", err)
	}
	s.say("writing %s and %s, regenerating compose and the lock", EnvImage, EnvDataVolume)
	// An interrupt must not stop the rewrite half way: the Applier holds to the end.
	if err := s.d.Applier.Apply(context.WithoutCancel(ctx), s.newEnv()); err != nil {
		return fmt.Errorf("switching env, compose and lock: %w", err)
	}
	return s.persist(PhaseSwitched)
}

// healthWait bounds the wait for postgres after the switch.
const healthWait = 3 * time.Minute

// start brings the whole stack up on the new volume and finishes.
func (s *Switch) start(ctx context.Context) error {
	s.say("starting the stack on %s", s.st.NewVolume)
	if err := s.d.Stack.Up(ctx); err != nil {
		return fmt.Errorf("starting the stack: %w", err)
	}
	wctx, cancel := context.WithTimeout(ctx, healthWait)
	defer cancel()
	if err := s.d.Stack.WaitHealthy(wctx, s.cfg.Service); err != nil {
		return fmt.Errorf("postgres did not become healthy on the new volume: %w", err)
	}
	if err := s.persist(PhaseDone); err != nil {
		return err
	}
	return s.cleanStage()
}
