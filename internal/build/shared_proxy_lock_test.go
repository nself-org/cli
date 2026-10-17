package build

// shared_proxy_lock_test.go — a shared-proxy build in write mode holds the
// owner lock (sharedproxy.Lock) while it writes; plan mode and --check never
// wait for it.

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/sharedproxy"
)

// buildDuringLock builds dir while another holder has the owner lock. It
// returns whether the build had finished 400ms into the hold, and its error
// once the lock was released.
func buildDuringLock(t *testing.T, x *sharedFx, dir string, opts BuildOptions) (finishedWhileHeld bool, err error) {
	t.Helper()
	release, lerr := sharedproxy.Lock(context.Background(), x.owner)
	if lerr != nil {
		t.Fatal(lerr)
	}
	done := make(chan error, 1)
	go func() {
		_, berr := Build(dir, opts)
		done <- berr
	}()
	select {
	case err = <-done:
		release()
		return true, err
	case <-time.After(400 * time.Millisecond):
	}
	release()
	select {
	case err = <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("the build never finished after the lock was released")
	}
	return false, err
}

func TestSharedProxyBuildTakesTheOwnerLock(t *testing.T) {
	x := newSharedFx(t)
	x.mustBuild(t, x.owner, x.ownerEnv())
	x.mustBuild(t, x.backend(), x.frontedEnv("bproj", "b.test"))

	t.Run("fronted write build waits", func(t *testing.T) {
		x.prepare(t, x.backend(), x.frontedEnv("bproj", "b.test"))
		finished, err := buildDuringLock(t, x, x.backend(), BuildOptions{Force: true})
		if finished || err != nil {
			t.Fatalf("a fronted build must wait for the owner lock (finished early: %v) and then succeed: %v", finished, err)
		}
	})
	t.Run("owner write build waits", func(t *testing.T) {
		x.prepare(t, x.owner, x.ownerEnv())
		finished, err := buildDuringLock(t, x, x.owner, BuildOptions{Force: true})
		if finished || err != nil {
			t.Fatalf("the owner's build must wait for the owner lock (finished early: %v) and then succeed: %v", finished, err)
		}
	})
	t.Run("plan mode does not wait", func(t *testing.T) {
		x.prepare(t, x.backend(), x.frontedEnv("bproj", "b.test"))
		finished, err := buildDuringLock(t, x, x.backend(), BuildOptions{Force: true, Mode: ModePlan})
		if !finished || err != nil {
			t.Fatalf("a plan writes nothing and must not wait (finished: %v): %v", finished, err)
		}
	})
	t.Run("check mode does not wait", func(t *testing.T) {
		x.prepare(t, x.backend(), x.frontedEnv("bproj", "b.test"))
		finished, err := buildDuringLock(t, x, x.backend(), BuildOptions{Force: true, Check: true})
		if !finished || err != nil {
			t.Fatalf("--check writes nothing and must not wait (finished: %v): %v", finished, err)
		}
	})
	t.Run("the lock is released when the build returns", func(t *testing.T) {
		x.mustBuild(t, x.backend(), x.frontedEnv("bproj", "b.test"))
		release, err := sharedproxy.Lock(context.Background(), x.owner)
		if err != nil {
			t.Fatal(err)
		}
		release()
	})
}
