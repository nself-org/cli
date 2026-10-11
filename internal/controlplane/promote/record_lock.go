package promote

// Purpose: Serialize promotion record eligibility and recovery with deployment.
// Inputs: Project root and the invocation context, optionally carrying its lock.
// Outputs: Locked context, release function and a clear contention error.
// Constraints: Reuse the existing project oplock; context re-entry avoids taking
// it twice. Host rollback locks are distinct and remain owned by RollbackRemote.

import (
	"context"
	"errors"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/oplock"
)

// AcquireRecordLock holds the deployment operation lock for the entire rollback.
func AcquireRecordLock(ctx context.Context, projectRoot string) (context.Context, func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if oplock.FromContext(ctx) != nil {
		return ctx, func() {}, nil
	}
	// Invocation re-entry is explicit in ctx. Do not accept a process-wide token:
	// independent goroutines must not impersonate the same lock owner.
	lock, err := oplock.Acquire(ctx, projectRoot, oplock.Opts{
		Command: "nself promote rollback", Getenv: func(string) string { return "" },
	})
	if errors.Is(err, oplock.ErrHeld) {
		return ctx, func() {}, errs.Wrap("E460", "another deploy operation holds the project lock; wait and retry promotion rollback", err)
	}
	if err != nil {
		return ctx, func() {}, err
	}
	return oplock.WithLock(ctx, lock), lock.Release, nil
}
