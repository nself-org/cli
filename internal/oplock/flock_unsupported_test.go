//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package oplock

import (
	"context"
	"syscall"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// TestOpLockGuardUnsupportedFilesystem pins the unsupported-flock exception to
// v1.5 fail-closed policy without relying on the host's filesystem type.
func TestOpLockGuardUnsupportedFilesystem(t *testing.T) {
	compattest.Set(t, true)
	useGuardEnv(t, newFake())
	old := flockCall
	t.Cleanup(func() { flockCall = old })
	for _, failure := range []error{syscall.ENOSYS, syscall.EOPNOTSUPP} {
		flockCall = func(int, int) error { return failure }
		ctx, release, err := Guard(context.Background(), Request{Dir: t.TempDir(), Command: "x", SideEffect: "write", Output: "document"})
		release()
		if err != nil || FromContext(ctx) != nil {
			t.Fatalf("unsupported filesystem must proceed unlocked (%v): %v", failure, err)
		}
	}
}
