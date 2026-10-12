package load

// Purpose: one import per target at a time.
// Inputs: a dedicated target connection. Outputs: E519 when another import holds the lock.
// Constraints: session-level advisory lock, released when the connection closes; never waits.
import (
	"context"

	"github.com/nself-org/cli/internal/errs"
)

// importLockKey is the fixed advisory-lock key ("nselfimp") shared by every import run.
const importLockKey int64 = 0x6e73656c66696d70

// TryLock takes the import lock on db without waiting.
func TryLock(ctx context.Context, db queryRower) error {
	var ok bool
	if db.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, importLockKey).Scan(&ok) != nil {
		return errs.New("E521", "target import lock failed")
	}
	if !ok {
		return errs.New("E519", "another import is running against this target")
	}
	return nil
}
