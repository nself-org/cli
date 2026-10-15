//go:build !windows

package oplock

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
)

// TestOpLockGuardV15OpenFailure refuses mutation when the lock path cannot open.
func TestOpLockGuardV15OpenFailure(t *testing.T) {
	compattest.Set(t, true)
	dir := t.TempDir()
	if err := os.MkdirAll(LockPath(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	stderr := useGuardEnv(t, newFake())
	ctx, release, err := Guard(context.Background(), Request{Dir: dir, Command: "x", SideEffect: "write", Output: "document"})
	release()
	d := errs.Describe(err)
	var pathErr *os.PathError
	if d == nil || d.Code != "E460" || errs.ExitCodeFor(err) != 1 || !errors.As(err, &pathErr) || FromContext(ctx) != nil {
		t.Fatalf("want E460 wrapping open failure and no lock; got %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("failure must not say it proceeds unlocked: %q", stderr.String())
	}
	if d.Cause != "The project operation lock could not be acquired safely." {
		t.Fatalf("I/O failure must not claim contention: %q", d.Cause)
	}
}
