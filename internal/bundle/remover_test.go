package bundle

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func TestRemove_UnknownBundle(t *testing.T) {
	_, err := Remove(context.Background(), "bogus", RemoveOpts{Out: &bytes.Buffer{}, DryRun: true})
	if err == nil {
		t.Fatal("expected error for unknown bundle")
	}
	if !strings.Contains(err.Error(), "unknown bundle") {
		t.Errorf("error %v missing 'unknown bundle'", err)
	}
}

func TestRemove_MetaBundleRejected(t *testing.T) {
	_, err := Remove(context.Background(), "nself-plus", RemoveOpts{Out: &bytes.Buffer{}, DryRun: true})
	if err == nil {
		t.Fatal("expected error for nself-plus")
	}
}

// TestRemove_DryRunListsInstalled creates fake plugin dirs for two of the
// bundle's plugins and verifies dry-run reports them as planned.
func TestRemove_DryRunListsInstalled(t *testing.T) {
	pluginDir := t.TempDir()
	// Pre-populate two plugins from the nsentry bundle.
	for _, p := range []string{"nself-uptime-monitor", "nself-status-page"} {
		if err := os.MkdirAll(filepath.Join(pluginDir, p), 0700); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	// P7-PLUG-20: remove plans from the ledger, so the bundle must be recorded.
	recordBundle(t, pluginDir, "sentry")
	var buf bytes.Buffer
	res, err := Remove(context.Background(), "nsentry", RemoveOpts{
		DryRun:    true,
		Out:       &buf,
		PluginDir: pluginDir,
	})
	if err != nil {
		t.Fatalf("dry-run remove failed: %v", err)
	}
	if len(res.Planned) != 2 {
		t.Errorf("planned %d; want 2 (only two were installed)", len(res.Planned))
	}
	if len(res.Removed) != 0 {
		t.Errorf("dry-run removed %d; want 0", len(res.Removed))
	}
	if !strings.Contains(buf.String(), "Remove plan (dry-run)") {
		t.Errorf("output missing dry-run header: %s", buf.String())
	}
}

// TestRemove_FullSuccess simulates a full bundle removal with the test hook.
func TestRemove_FullSuccess(t *testing.T) {
	pluginDir := t.TempDir()
	bundlePlugins := []string{"nself-uptime-monitor", "nself-status-page", "nself-incident-mgmt"}
	for _, p := range bundlePlugins {
		if err := os.MkdirAll(filepath.Join(pluginDir, p), 0700); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	recordBundle(t, pluginDir, "sentry") // P7-PLUG-20: remove plans from the ledger
	var calls []string
	removeFn := func(ctx context.Context, cfg *config.Config, name, pd string, kd, force bool) error {
		calls = append(calls, name)
		// Honour keepData==false by removing the dir.
		return os.RemoveAll(filepath.Join(pd, name))
	}

	var buf bytes.Buffer
	res, err := Remove(context.Background(), "nsentry", RemoveOpts{
		Out:       &buf,
		PluginDir: pluginDir,
		remover:   removeFn,
	})
	if err != nil {
		t.Fatalf("Remove failed: %v\n%s", err, buf.String())
	}
	// At least the three pre-populated should have been removed.
	if len(res.Removed) < 3 {
		t.Errorf("removed %d; want >= 3", len(res.Removed))
	}
	// Calls should be in reverse plugin-list order. The bundle lists
	// nself-uptime-monitor first; removal should hit it LAST among the three.
	if len(calls) < 3 {
		t.Fatalf("only %d remove calls", len(calls))
	}
	if calls[len(calls)-1] != "nself-uptime-monitor" {
		t.Errorf("expected nself-uptime-monitor removed last; got order %v", calls)
	}
}

// TestRemove_KeepDataPropagates verifies the keepData flag reaches plugin.Remove.
func TestRemove_KeepDataPropagates(t *testing.T) {
	pluginDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(pluginDir, "nself-uptime-monitor"), 0700); err != nil {
		t.Fatalf("setup: %v", err)
	}
	recordBundle(t, pluginDir, "sentry") // P7-PLUG-20: remove plans from the ledger
	var gotKeepData bool
	removeFn := func(ctx context.Context, cfg *config.Config, name, pd string, kd, force bool) error {
		if name == "nself-uptime-monitor" {
			gotKeepData = kd
		}
		return os.RemoveAll(filepath.Join(pd, name))
	}
	var buf bytes.Buffer
	_, err := Remove(context.Background(), "nsentry", RemoveOpts{
		KeepData:  true,
		Out:       &buf,
		PluginDir: pluginDir,
		remover:   removeFn,
	})
	if err != nil {
		t.Fatalf("Remove failed: %v\n%s", err, buf.String())
	}
	if !gotKeepData {
		t.Error("--keep-data did not propagate to plugin.Remove")
	}
	if !strings.Contains(buf.String(), "data: preserve") {
		t.Errorf("output missing keep-data banner: %s", buf.String())
	}
}

// TestRemove_PartialFailure verifies that a per-plugin failure STOPS the remaining
// removals (P7-PLUG-20: the old continue-on-failure left a bundle half removed
// with no record of where) and is surfaced as an error with the failed name.
func TestRemove_PartialFailure(t *testing.T) {
	pluginDir := t.TempDir()
	for _, p := range []string{"nself-uptime-monitor", "nself-status-page", "nself-incident-mgmt"} {
		if err := os.MkdirAll(filepath.Join(pluginDir, p), 0700); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	recordBundle(t, pluginDir, "sentry")
	failed := "nself-status-page"
	removeFn := func(ctx context.Context, cfg *config.Config, name, pd string, kd, force bool) error {
		if name == failed {
			return errors.New("synthetic remove failure")
		}
		return os.RemoveAll(filepath.Join(pd, name))
	}
	var buf bytes.Buffer
	res, err := Remove(context.Background(), "nsentry", RemoveOpts{
		Out:       &buf,
		PluginDir: pluginDir,
		remover:   removeFn,
	})
	if err == nil {
		t.Fatal("expected aggregated failure")
	}
	if !strings.Contains(err.Error(), failed) {
		t.Errorf("error missing failed plugin name: %v", err)
	}
	// Reverse order is incident-mgmt, status-page, uptime-monitor: the first
	// is removed, the second fails, the third is never attempted.
	if len(res.Removed) != 1 || res.Removed[0] != "nself-incident-mgmt" {
		t.Errorf("Removed = %v; want [nself-incident-mgmt]", res.Removed)
	}
	if _, statErr := os.Stat(filepath.Join(pluginDir, "nself-uptime-monitor")); statErr != nil {
		t.Errorf("removal continued past the failure: %v", statErr)
	}
	if len(res.Failed) != 1 || res.Failed[0] != failed {
		t.Errorf("Failed = %v; want [%s]", res.Failed, failed)
	}
}
