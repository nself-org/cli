package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/bundle/appready"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/health"
	"github.com/nself-org/cli/internal/reconcile"

	"github.com/spf13/cobra"
)

func countReconciles(t *testing.T) *int {
	t.Helper()
	n := 0
	old := bundleReconcile
	bundleReconcile = func(*cobra.Command, reconcile.Trigger) error { n++; return nil }
	t.Cleanup(func() { bundleReconcile = old })
	return &n
}

func stubInstall(t *testing.T, res *bundle.InstallResult, got *bundle.InstallOpts) {
	t.Helper()
	old := bundleInstall
	bundleInstall = func(_ context.Context, _ string, opts bundle.InstallOpts) (*bundle.InstallResult, error) {
		if got != nil {
			*got = opts
		}
		return res, nil
	}
	t.Cleanup(func() { bundleInstall = old })
}

// TestBundleInstallReconcilesOnce: when Install already ran the reconcile hook
// the command layer must not reconcile again; when it did not (rollback path,
// legacy), it does.
func TestBundleInstallReconcilesOnce(t *testing.T) {
	n := countReconciles(t)
	stubInstall(t, &bundle.InstallResult{Changed: true, Reconciled: true}, nil)
	if err := runBundleInstall(bundleInstallCmd, []string{"task"}); err != nil || *n != 0 {
		t.Fatalf("already reconciled by the hook: err=%v reconciles=%d, want 0", err, *n)
	}
	stubInstall(t, &bundle.InstallResult{Changed: true, Reconciled: false}, nil)
	if err := runBundleInstall(bundleInstallCmd, []string{"task"}); err != nil || *n != 1 {
		t.Fatalf("not reconciled yet: err=%v reconciles=%d, want 1", err, *n)
	}
	stubInstall(t, &bundle.InstallResult{Changed: false}, nil)
	if err := runBundleInstall(bundleInstallCmd, []string{"task"}); err != nil || *n != 1 {
		t.Fatalf("unchanged: err=%v reconciles=%d, want still 1", err, *n)
	}
}

// TestBundleInstallPassesHooksExceptDryRun: hooks are wired for a real install
// only; a dry run changes nothing and must not carry hooks.
func TestBundleInstallPassesHooksExceptDryRun(t *testing.T) {
	var got bundle.InstallOpts
	stubInstall(t, &bundle.InstallResult{}, &got)
	if err := runBundleInstall(bundleInstallCmd, []string{"task"}); err != nil || got.Hooks == nil {
		t.Fatalf("real install: err=%v hooks=%v", err, got.Hooks)
	}
	if err := bundleInstallCmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bundleInstallCmd.Flags().Set("dry-run", "false") })
	if err := runBundleInstall(bundleInstallCmd, []string{"task"}); err != nil || got.Hooks != nil || !got.DryRun {
		t.Fatalf("dry run: err=%v hooks=%v dry=%v", err, got.Hooks, got.DryRun)
	}
}

// TestBundleHooksOutsideProject: reconcile reports ErrNoProject, never reconciles.
func TestBundleHooksOutsideProject(t *testing.T) {
	compattest.Set(t, true) // the metadata hook exists only in v1.5
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	n := countReconciles(t)
	h := newBundleAppReadyHooks(bundleInstallCmd, "task")
	if err := h.Reconcile(context.Background()); !errors.Is(err, appready.ErrNoProject) || *n != 0 {
		t.Fatalf("Reconcile outside a project: err=%v reconciles=%d", err, *n)
	}
	if _, err := h.ExportMetadata(context.Background()); !errors.Is(err, appready.ErrNoProject) {
		t.Errorf("ExportMetadata outside a project: %v", err)
	}
}

// TestBundleHooksInProject: reconcile runs, metadata and health go through the seams.
func TestBundleHooksInProject(t *testing.T) {
	compattest.Set(t, true) // start, metadata and health hooks exist only in v1.5
	t.Setenv("HOME", t.TempDir())
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, ".env"), []byte("PROJECT_NAME=hk\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj)
	n := countReconciles(t)
	oldMeta, oldHealth := bundleExportMetadata, bundleRunHealth
	t.Cleanup(func() { bundleExportMetadata, bundleRunHealth = oldMeta, oldHealth })
	bundleExportMetadata = func(context.Context, *config.Config) ([]byte, error) { return []byte(`{"sources":[]}`), nil }
	h := newBundleAppReadyHooks(bundleInstallCmd, "task")
	if err := h.Reconcile(context.Background()); err != nil || *n != 1 {
		t.Fatalf("Reconcile: err=%v reconciles=%d", err, *n)
	}
	if raw, err := h.ExportMetadata(context.Background()); err != nil || !strings.Contains(string(raw), "sources") {
		t.Fatalf("ExportMetadata: %s %v", raw, err)
	}
	bundleRunHealth = func(context.Context, *config.Config, string) (*health.HealthReport, error) {
		return &health.HealthReport{Results: []health.HealthResult{{Service: "hasura", Status: "healthy"}, {Service: "jobs", Status: "unhealthy"}}}, nil
	}
	err := h.ServiceHealth(context.Background())
	if err == nil || !strings.Contains(err.Error(), "jobs (unhealthy)") || strings.Contains(err.Error(), "hasura") {
		t.Fatalf("ServiceHealth = %v, want only jobs listed", err)
	}
	bundleRunHealth = func(context.Context, *config.Config, string) (*health.HealthReport, error) {
		return &health.HealthReport{Results: []health.HealthResult{{Service: "hasura", Status: "running"}}}, nil
	}
	if err := h.ServiceHealth(context.Background()); err != nil {
		t.Errorf("all healthy: %v", err)
	}
}
