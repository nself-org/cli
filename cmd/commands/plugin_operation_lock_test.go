package commands

// D-0795: a separate process must be excluded throughout plugin mutation,
// including downloads and the final ledger mark, then admitted on return.
import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/spf13/cobra"
)

const pluginProbeDir = "NSELF_PLUGIN_OPERATION_PROBE_DIR"

func TestPluginOperationLockProbe(t *testing.T) {
	dir := os.Getenv(pluginProbeDir)
	if dir == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := bundle.LedgerStore(dir).Operation(ctx, "plugin probe", func() error { return nil })
	if errors.Is(err, context.DeadlineExceeded) {
		os.Exit(3)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func probePluginOperationLock(t *testing.T, dir string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPluginOperationLockProbe$", "-test.timeout=4s")
	cmd.Env = append(os.Environ(), pluginProbeDir+"="+dir)
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("probe timed out: %v", ctx.Err())
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	return 0
}

func TestPluginInstallHoldsOperationLock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native Windows has no file locks")
	}
	compattest.Both(t, func(t *testing.T) {
		p := extensionFixture(t, "")
		downloads := make(chan int, 4)
		localPluginRegistry(t, func() { downloads <- probePluginOperationLock(t, p.plugins) })
		oldTrack := pluginTrack
		pluginTrack = func(_ *cobra.Command, dir string, _ []string) error {
			if code := probePluginOperationLock(t, dir); code != 3 {
				t.Errorf("final ledger mark: probe exit %d, want 3 (busy)", code)
			}
			return nil
		}
		t.Cleanup(func() { pluginTrack = oldTrack })
		if err := runPluginInstall(pluginInstallCmd, []string{"zzfixture"}); err != nil {
			t.Fatal(err)
		}
		select {
		case code := <-downloads:
			if code != 3 {
				t.Errorf("download: probe exit %d, want 3 (busy)", code)
			}
		default:
			t.Fatal("install did not download the fixture")
		}
		if code := probePluginOperationLock(t, p.plugins); code != 0 {
			t.Errorf("after install: probe exit %d, want 0", code)
		}
	})
}

func TestPluginRemoveHoldsOperationLock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native Windows has no file locks")
	}
	compattest.Both(t, func(t *testing.T) {
		p := extensionFixture(t, "")
		stageWritePlugin(t, p.plugins, "solo")
		oldRemove := pluginRemoveFn
		pluginRemoveFn = func(_ context.Context, _ *config.Config, _, dir string, _, _ bool) error {
			if code := probePluginOperationLock(t, dir); code != 3 {
				t.Errorf("removal: probe exit %d, want 3 (busy)", code)
			}
			return nil
		}
		t.Cleanup(func() { pluginRemoveFn = oldRemove })
		setFlags(t, removeCmd, "keep-data")
		t.Cleanup(func() { resetFlag(removeCmd, "keep-data") })
		if err := runPluginRemove(removeCmd, []string{"solo"}); err != nil {
			t.Fatal(err)
		}
		if code := probePluginOperationLock(t, p.plugins); code != 0 {
			t.Errorf("after remove: probe exit %d, want 0", code)
		}
	})
}
