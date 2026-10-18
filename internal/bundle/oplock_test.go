package bundle

// D-0322: bundle install and bundle remove hold the install/remove operation lock
// (ledger/oplock.go) across their member loops. A second process (the test binary
// re-executed as a probe) tries to take the lock from inside the member hook: it
// must find it busy, and must get it once the command has returned.

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/config"
)

const envProbeDir = "NSELF_BUNDLE_OPLOCK_PROBE_DIR"

// Timing budgets (D-0850). A probe re-executes the test binary, so on a loaded
// gate host its start-up alone can take seconds; the budgets only catch hangs.
const (
	opLockProbeTimeout      = 20 * time.Second
	opLockProbeChildTimeout = 15 * time.Second
	opLockGuardTimeout      = 6 * time.Minute
)

// TestBundleOpLockProbeProcess is the child: it does nothing unless envProbeDir
// is set. Exit 0: the lock was acquired. Exit 3: still busy after 200 ms.
func TestBundleOpLockProbeProcess(t *testing.T) {
	dir := os.Getenv(envProbeDir)
	if dir == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := ledger.NewStore(dir).Operation(ctx, "probe", func() error { return nil })
	switch {
	case err == nil:
		os.Exit(0)
	case errors.Is(err, context.DeadlineExceeded):
		os.Exit(3)
	}
	os.Exit(4)
}

// probeOpLock runs the probe child against pluginDir and returns its exit code.
func probeOpLock(t *testing.T, pluginDir string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), opLockProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBundleOpLockProbeProcess$", "-test.timeout="+opLockProbeChildTimeout.String())
	cmd.Env = append(os.Environ(), envProbeDir+"="+pluginDir)
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("operation lock probe timed out: %v", ctx.Err())
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	if err != nil {
		t.Fatalf("probe did not run: %v", err)
	}
	return 0
}

func needsOSLock(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("no OS file lock on native Windows; WSL2 runs the unix build")
	}
}

func guardOperationLockTest(t *testing.T) {
	t.Helper()
	guard := time.AfterFunc(opLockGuardTimeout, func() { panic("operation lock test timed out: " + t.Name()) })
	t.Cleanup(func() { guard.Stop() })
}

func TestInstallHoldsOperationLockDuringMemberLoop(t *testing.T) {
	needsOSLock(t)
	guardOperationLockTest(t)
	setupOfflineRegistry(t, mockNsentryRegistry())
	pluginDir := t.TempDir()
	var during []int
	installFn := func(_ context.Context, _ *config.Config, name, pd string) error {
		during = append(during, probeOpLock(t, pd))
		return os.MkdirAll(filepath.Join(pd, name), 0o700)
	}
	var buf bytes.Buffer
	res, err := Install(context.Background(), "nsentry", InstallOpts{
		Out:                   &buf,
		Channel:               ChannelStable,
		PluginDir:             pluginDir,
		Config:                &config.Config{},
		bundleEntitledChecker: func(_ context.Context, _, _ string) (bool, error) { return true, nil },
		licenseChecker:        func(_ context.Context, _ []string) error { return nil },
		installer:             installFn,
	})
	if err != nil {
		t.Fatalf("Install: %v\n%s", err, buf.String())
	}
	if len(res.Installed) == 0 || len(during) != len(res.Installed) {
		t.Fatalf("installed %v, probed %v", res.Installed, during)
	}
	for i, code := range during {
		if code != 3 {
			t.Errorf("member %d: probe exit %d while installing; want 3 (busy)", i, code)
		}
	}
	if code := probeOpLock(t, pluginDir); code != 0 {
		t.Errorf("probe exit %d after Install returned; want 0 (acquired)", code)
	}
}

func TestRemoveHoldsOperationLockDuringRemovals(t *testing.T) {
	needsOSLock(t)
	guardOperationLockTest(t)
	pluginDir := t.TempDir()
	for _, p := range []string{"nself-uptime-monitor", "nself-status-page", "nself-incident-mgmt"} {
		if err := os.MkdirAll(filepath.Join(pluginDir, p), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	recordBundle(t, pluginDir, "sentry")
	var during []int
	removeFn := func(_ context.Context, _ *config.Config, name, pd string, _, _ bool) error {
		during = append(during, probeOpLock(t, pd))
		return os.RemoveAll(filepath.Join(pd, name))
	}
	var buf bytes.Buffer
	res, err := Remove(context.Background(), "nsentry", RemoveOpts{
		Out:       &buf,
		PluginDir: pluginDir,
		Config:    &config.Config{},
		remover:   removeFn,
	})
	if err != nil {
		t.Fatalf("Remove: %v\n%s", err, buf.String())
	}
	if len(res.Removed) == 0 || len(during) != len(res.Removed) {
		t.Fatalf("removed %v, probed %v", res.Removed, during)
	}
	for i, code := range during {
		if code != 3 {
			t.Errorf("member %d: probe exit %d while removing; want 3 (busy)", i, code)
		}
	}
	if code := probeOpLock(t, pluginDir); code != 0 {
		t.Errorf("probe exit %d after Remove returned; want 0 (acquired)", code)
	}
}

// TestRemoveDryRunTakesNoOperationLock: a dry run is read only and never waits.
func TestRemoveDryRunTakesNoOperationLock(t *testing.T) {
	needsOSLock(t)
	guardOperationLockTest(t)
	pluginDir := t.TempDir()
	recordBundle(t, pluginDir, "sentry")
	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- ledger.NewStore(pluginDir).Operation(context.Background(), "test hold", func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	defer func() { close(release); <-done }()
	if _, err := Remove(context.Background(), "nsentry", RemoveOpts{DryRun: true, Out: &bytes.Buffer{}, PluginDir: pluginDir}); err != nil {
		t.Fatalf("dry-run Remove while the lock is held: %v", err)
	}
}
