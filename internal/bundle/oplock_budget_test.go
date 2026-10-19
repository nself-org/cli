package bundle

// D-0850: the install/remove operation-lock tests re-execute the test binary once
// per bundle member. Under the loaded gate each probe took long enough that the
// old whole-test guard (5s) fired before a five-member bundle finished. The guard
// is only a deadlock net, so it must outlast every probe the tests can run.
//
// A lock that is never released made the macOS gate hang for the whole 10 minute
// `go test` timeout (cam CI, 2026-10-13). opCtx gives every Install and Remove
// call in the tests a deadline, so a stuck lock fails that one test with a
// context error in seconds instead of stalling the package.

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/bundle/ledger"
)

// maxOpLockProbes is the most probes one operation-lock test runs: a bundle's
// members plus the final "acquired" probe, with headroom for bigger bundles.
const maxOpLockProbes = 16

// opCallTimeout bounds one Install or Remove call in a test that uses opCtx. The
// calls use fakes and finish in well under a second; this only catches a lock
// that is never released.
const opCallTimeout = 30 * time.Second

// envHoldDir makes TestBundleOpLockHolderProcess hold the operation lock of that
// plugin directory until its stdin closes.
const envHoldDir = "NSELF_BUNDLE_OPLOCK_HOLD_DIR"

// opCtx returns a context for one Install or Remove call: it expires after
// opCallTimeout, so a stuck operation lock fails the test fast. The waiter in
// ledger.Store.Operation stops on context cancel and wraps ctx.Err().
func opCtx(t testing.TB) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), opCallTimeout)
	t.Cleanup(cancel)
	return ctx
}

func TestOperationLockGuardOutlastsProbes(t *testing.T) {
	if need := maxOpLockProbes * opLockProbeTimeout; opLockGuardTimeout < need {
		t.Fatalf("guard %v fires before %d probes of up to %v each (%v)", opLockGuardTimeout, maxOpLockProbes, opLockProbeTimeout, need)
	}
	if opLockProbeChildTimeout >= opLockProbeTimeout {
		t.Fatalf("child -test.timeout %v must stay under the parent probe timeout %v", opLockProbeChildTimeout, opLockProbeTimeout)
	}
	if opCallTimeout >= opLockGuardTimeout {
		t.Fatalf("per-call deadline %v must fire before the whole-test guard %v", opCallTimeout, opLockGuardTimeout)
	}
}

// TestBundleOpLockHolderProcess is the child of TestInstallFailsFastWhenLockStuck:
// it does nothing unless envHoldDir is set. It takes the operation lock, prints
// "held", and keeps the lock until its stdin is closed.
func TestBundleOpLockHolderProcess(t *testing.T) {
	dir := os.Getenv(envHoldDir)
	if dir == "" {
		return
	}
	err := ledger.NewStore(dir).Operation(context.Background(), "holder", func() error {
		os.Stdout.WriteString("held\n")
		_, _ = bufio.NewReader(os.Stdin).ReadByte() // returns at EOF, when the parent closes stdin
		return nil
	})
	if err != nil {
		os.Exit(4)
	}
	os.Exit(0)
}

// TestInstallFailsFastWhenLockStuck: another process holds the operation lock and
// never lets go. Install with a short deadline must return an error that wraps
// context.DeadlineExceeded within seconds, not wait on the lock for ever.
func TestInstallFailsFastWhenLockStuck(t *testing.T) {
	needsOSLock(t)
	guardOperationLockTest(t)
	pluginDir := isolate(t)
	setupOfflineRegistry(t, mockNotificationsOnly())

	ctx, cancelHolder := context.WithTimeout(context.Background(), opLockProbeTimeout)
	defer cancelHolder()
	holder := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBundleOpLockHolderProcess$", "-test.timeout="+opLockProbeChildTimeout.String())
	holder.Env = append(os.Environ(), envHoldDir+"="+pluginDir)
	stdin, err := holder.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := holder.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := holder.Start(); err != nil {
		t.Fatalf("start lock holder: %v", err)
	}
	defer func() { stdin.Close(); _ = holder.Wait() }()
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "held\n" {
		t.Fatalf("lock holder did not report the lock held: %q %v", line, err)
	}

	short, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = Install(short, "task", freeOpts(t, pluginDir, &bytes.Buffer{}, func(string) error { return nil }))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("install = %v, want an error wrapping context.DeadlineExceeded", err)
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("install took %v to give up on a stuck lock, want seconds", took)
	}
}
