package commands

// Purpose: bound rolling compose up and describe interrupted restarts.
// Inputs: deploy context, resolved rolling plan and service name.
// Outputs: an error preserving cancellation, timeout or Docker's failure.
// Constraints: only internal/docker launches Docker; no automatic recovery.

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

const rollingUpTimeout = 10 * time.Minute

// rollingUp applies one service with a separate, generous deadline. Recovery
// is manual because Compose may already have stopped the old container.
func rollingUp(ctx context.Context, plan rollingPlan, svc string) error {
	limit := plan.upTimeout
	if limit <= 0 {
		limit = rollingUpTimeout
	}
	upCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	err := plan.compose.ComposeUpNoDeps(upCtx, plan.workdir, svc)
	if err == nil {
		return nil
	}
	recovery := fmt.Sprintf("Service %s may be stopped. Run 'nself start %s' to recover", svc, svc)
	if ctx.Err() != nil {
		return fmt.Errorf("rolling restart interrupted during up of service %s: %w\n%s", svc, ctx.Err(), recovery)
	}
	if upCtx.Err() != nil {
		return fmt.Errorf("rolling restart: service %s up timed out after %s: %w\n%s", svc, limit, upCtx.Err(), recovery)
	}
	err = rollingInterrupt(err)
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("rolling restart interrupted during up of service %s: %w\n%s", svc, err, recovery)
	}
	return fmt.Errorf("rolling restart: service %s restart failed: %w\nRun 'nself logs %s' for details", svc, err, svc)
}

// rollingInterrupt recognizes a Docker child that handled Ctrl-C before the
// parent's signal handler canceled the context. Preserve the original error.
func rollingInterrupt(err error) error {
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 130 {
		return fmt.Errorf("%w: %w", context.Canceled, err)
	}
	return err
}
