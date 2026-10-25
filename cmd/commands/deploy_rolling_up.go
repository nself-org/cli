package commands

// Purpose: the bounded `up -d --no-deps` step of the rolling restart (D-0469).
// Inputs: a rollingPlan and one service name. Outputs: nil, or an error that
// says whether docker timed out, was interrupted, or failed.
// Constraints: every docker call goes through internal/docker.

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// rollingUpTimeout bounds one `up -d --no-deps` in the rolling path. It is
// generous because `up` can pull an image and recreate the container before it
// returns; the health gate has its own, shorter limit. A var so tests can shorten it.
var rollingUpTimeout = 15 * time.Minute

// exitInterrupted is the status a docker child gives back when it saw SIGINT.
const exitInterrupted = 130

// rollingUp brings svc up under rollingUpTimeout. A daemon that hangs during
// `up` ends in a named error instead of hanging the deploy, and an interrupted
// `up` says the service may be left stopped and how to bring it back, because
// `up` stops the old container before it starts the new one.
func rollingUp(ctx context.Context, plan rollingPlan, svc string) error {
	upCtx, cancel := context.WithTimeout(ctx, rollingUpTimeout)
	defer cancel()
	err := plan.compose.ComposeUpNoDeps(upCtx, plan.workdir, svc)
	if err == nil {
		return nil
	}
	hint := fmt.Sprintf("Service %s may be stopped: run 'nself restart %s' to bring it back", svc, svc)
	var exit *exec.ExitError
	switch {
	case ctx.Err() != nil:
		return fmt.Errorf("rolling restart interrupted while bringing up service %s: %w. %s", svc, ctx.Err(), hint)
	case errors.As(err, &exit) && exit.ExitCode() == exitInterrupted:
		// docker saw the Ctrl-C before nself's own handler cancelled ctx.
		return fmt.Errorf("rolling restart interrupted while bringing up service %s: %w. %s", svc, context.Canceled, hint)
	case upCtx.Err() != nil:
		return fmt.Errorf("rolling restart: service %s did not finish 'up' within %s, so docker was stopped. %s", svc, rollingUpTimeout, hint)
	}
	return fmt.Errorf("rolling restart: service %s restart failed: %w\nRun 'nself logs %s' for details", svc, err, svc)
}
