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

// stubPlanReturn forces ResolveBundleVersions failure to a stable path by
// using a tiny bundle slug and overriding internal hooks. We need to bypass
// ResolveBundleVersions which hits the real network/cache; the test strategy
// is to set NSELF_PLUGIN_REGISTRY to a non-existent file and verify dry-run
// short-circuits before the install loop runs.

func TestInstall_UnknownBundle(t *testing.T) {
	ctx := context.Background()
	_, err := Install(ctx, "bogus", InstallOpts{DryRun: true, Out: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("expected error for unknown bundle")
	}
	if !strings.Contains(err.Error(), "unknown bundle") {
		t.Errorf("error %v does not mention 'unknown bundle'", err)
	}
}

func TestInstall_MetaBundleNotInstallable(t *testing.T) {
	ctx := context.Background()
	_, err := Install(ctx, "nself-plus", InstallOpts{DryRun: true, Out: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("expected error: nself-plus is not installable")
	}
	if !strings.Contains(err.Error(), "not installable") {
		t.Errorf("error %v missing 'not installable'", err)
	}
	if !strings.Contains(err.Error(), "nself bundle install claw") {
		t.Errorf("error %v should point at a paid bundle to install instead, got: %v", err, err)
	}
}

// TestInstall_TaskBundleNotInstallable_ListsPerPluginCommands proves the free
// Task Bundle's refusal is actionable: design-confirmed (Ruling 2 +
// P6-E4-W3-S3-T10 — free bundles install their plugins individually, not as
// a unit), the error must name the exact `nself plugin install <name>`
// command for every plugin in bundles.json's task.plugins list rather than a
// bare "not installable" dead end.
func TestInstall_TaskBundleNotInstallable_ListsPerPluginCommands(t *testing.T) {
	ctx := context.Background()
	_, err := Install(ctx, "task", InstallOpts{DryRun: true, Out: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("expected error: task is not installable as a unit")
	}
	if !strings.Contains(err.Error(), "not installable") {
		t.Errorf("error %v missing 'not installable'", err)
	}
	taskBundle, ok := Get("task")
	if !ok {
		t.Fatal("fixture missing task bundle")
	}
	if len(taskBundle.Plugins) == 0 {
		t.Fatal("fixture task bundle has no plugins to assert against")
	}
	for _, p := range taskBundle.Plugins {
		want := "nself plugin install " + p
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing per-plugin command %q; got: %v", want, err)
		}
	}
}

func TestInstall_DryRunWithMockedRegistry(t *testing.T) {
	dir := setupOfflineRegistry(t, mockNsentryRegistry())
	defer func() { _ = os.RemoveAll(dir) }()

	var buf bytes.Buffer
	res, err := Install(context.Background(), "nsentry", InstallOpts{
		DryRun:  true,
		Out:     &buf,
		Channel: ChannelStable,
	})
	if err != nil {
		t.Fatalf("dry-run Install failed: %v", err)
	}
	if !res.DryRun {
		t.Error("result.DryRun = false; want true")
	}
	if len(res.Installed) != 0 {
		t.Errorf("dry-run installed %d plugins; want 0", len(res.Installed))
	}
	if !strings.Contains(buf.String(), "Install plan (dry-run)") {
		t.Errorf("dry-run output missing header: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "nself-uptime-monitor@") {
		t.Errorf("dry-run output missing version pin: %s", buf.String())
	}
}

func TestInstall_AtomicRollbackOnFailure(t *testing.T) {
	dir := setupOfflineRegistry(t, mockNsentryRegistry())
	defer func() { _ = os.RemoveAll(dir) }()

	pluginDir := t.TempDir()
	var buf bytes.Buffer

	// Install hook: succeed for the first two plugins, fail on the third.
	failedAt := "nself-incident-mgmt"
	var installed []string
	installFn := func(ctx context.Context, cfg *config.Config, name, pd string) error {
		if name == failedAt {
			return errors.New("synthetic failure")
		}
		// Create the plugin dir to satisfy isAlreadyInstalled in rollback.
		if err := os.MkdirAll(filepath.Join(pd, name), 0700); err != nil {
			return err
		}
		installed = append(installed, name)
		return nil
	}
	var removed []string
	removeFn := func(ctx context.Context, cfg *config.Config, name, pd string) error {
		removed = append(removed, name)
		return os.RemoveAll(filepath.Join(pd, name))
	}

	res, err := Install(context.Background(), "nsentry", InstallOpts{
		Out:                   &buf,
		Channel:               ChannelStable,
		PluginDir:             pluginDir,
		bundleEntitledChecker: func(_ context.Context, _, _ string) (bool, error) { return true, nil }, // test: pass phase 1
		licenseChecker:        func(_ context.Context, _ []string) error { return nil },                // test: pass phase 2
		installer:             installFn,
		remover:               removeFn,
	})
	if err == nil {
		t.Fatalf("expected install failure; got nil err. output:\n%s", buf.String())
	}
	if len(res.Installed) == 0 {
		t.Error("expected some plugins installed before failure")
	}
	if len(res.RolledBack) != len(res.Installed) {
		t.Errorf("rolled back %d but %d were installed: %v vs %v", len(res.RolledBack), len(res.Installed), res.RolledBack, res.Installed)
	}
	// Rollback order should be reverse of install order.
	for i := range res.RolledBack {
		want := res.Installed[len(res.Installed)-1-i]
		if res.RolledBack[i] != want {
			t.Errorf("rollback order: got %q at index %d, want %q", res.RolledBack[i], i, want)
		}
	}
}

// TestReviewerUpgradeRollbackChangesMembership covers an upgrade followed by
// failure of the next plugin. Rollback must preserve the installed old version.
func TestReviewerUpgradeRollbackChangesMembership(t *testing.T) {
	setupOfflineRegistry(t, mockNsentryRegistry())
	pluginDir := t.TempDir()
	name := "nself-uptime-monitor"
	oldDir := filepath.Join(pluginDir, name)
	if err := os.MkdirAll(oldDir, 0700); err != nil {
		t.Fatal(err)
	}
	oldManifest := []byte(`{"name":"nself-uptime-monitor","version":"0.9.0","description":"Old version","category":"monitoring","license":"MIT"}`)
	if err := os.WriteFile(filepath.Join(oldDir, "plugin.json"), oldManifest, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "old-data"), []byte("preserve me"), 0600); err != nil {
		t.Fatal(err)
	}
	res, err := Install(context.Background(), "nsentry", InstallOpts{
		PluginDir: pluginDir,
		Out:       &bytes.Buffer{},
		bundleEntitledChecker: func(context.Context, string, string) (bool, error) {
			return true, nil
		},
		licenseChecker: func(context.Context, []string) error { return nil },
		installer: func(_ context.Context, _ *config.Config, pluginName, pd string) error {
			if pluginName == "nself-status-page" {
				return errors.New("injected second install failure")
			}
			dir := filepath.Join(pd, pluginName)
			if err := os.MkdirAll(dir, 0700); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"name":"`+pluginName+`","version":"1.0.0","description":"New version","category":"monitoring","license":"MIT"}`), 0600)
		},
		remover: func(_ context.Context, _ *config.Config, pluginName, pd string) error {
			return os.RemoveAll(filepath.Join(pd, pluginName))
		},
	})
	if err == nil || res == nil {
		t.Fatalf("expected second plugin failure, got result=%+v err=%v", res, err)
	}
	got, readErr := os.ReadFile(filepath.Join(oldDir, "plugin.json"))
	if readErr != nil || !bytes.Equal(got, oldManifest) {
		t.Errorf("old version must be restored: manifest=%q err=%v", got, readErr)
	}
	if data, readErr := os.ReadFile(filepath.Join(oldDir, "old-data")); readErr != nil || string(data) != "preserve me" {
		t.Errorf("old plugin files lost: data=%q err=%v", data, readErr)
	}
	if res.Changed {
		t.Errorf("restored plugin set should not need reconcile: %+v", res)
	}
}

// TestInstall_ForceStillValidatesLicense verifies that --force does NOT bypass
// license validation. It skips same-version checks (repair path) but license
// is always enforced.
func TestInstall_ForceStillValidatesLicense(t *testing.T) {
	dir := setupOfflineRegistry(t, mockNsentryRegistry())
	defer func() { _ = os.RemoveAll(dir) }()

	pluginDir := t.TempDir()
	var buf bytes.Buffer

	// License checker that returns an error — with --force, install must still
	// fail because force never bypasses the license gate.
	licenseFn := func(ctx context.Context, plugins []string) error {
		return errors.New("no license key configured")
	}
	installFn := func(ctx context.Context, cfg *config.Config, name, pd string) error {
		return os.MkdirAll(filepath.Join(pd, name), 0700)
	}
	removeFn := func(ctx context.Context, cfg *config.Config, name, pd string) error {
		return nil
	}

	_, err := Install(context.Background(), "nsentry", InstallOpts{
		Force:     true,
		Out:       &buf,
		Channel:   ChannelStable,
		PluginDir: pluginDir,
		// Phase 1: bundle entitled check returns false (simulates no key).
		bundleEntitledChecker: func(_ context.Context, _, _ string) (bool, error) {
			return false, errors.New("no license key configured")
		},
		licenseChecker: licenseFn,
		installer:      installFn,
		remover:        removeFn,
	})
	if err == nil {
		t.Fatal("expected license validation failure even with --force; got nil")
	}
	if !strings.Contains(err.Error(), "license validation failed") {
		t.Errorf("unexpected error (want 'license validation failed'): %v", err)
	}
	// Output must NOT claim license was bypassed.
	if strings.Contains(buf.String(), "bypassed license validation") {
		t.Error("output incorrectly claims license was bypassed with --force")
	}
}

func TestInstall_LicenseFailWithoutForce(t *testing.T) {
	dir := setupOfflineRegistry(t, mockNsentryRegistry())
	defer func() { _ = os.RemoveAll(dir) }()

	pluginDir := t.TempDir()
	var buf bytes.Buffer

	licenseFn := func(ctx context.Context, plugins []string) error {
		return errors.New("no license key configured")
	}

	res, err := Install(context.Background(), "nsentry", InstallOpts{
		Out:            &buf,
		Channel:        ChannelStable,
		PluginDir:      pluginDir,
		licenseChecker: licenseFn,
	})
	if err == nil {
		t.Fatal("expected license validation failure")
	}
	if len(res.Installed) != 0 {
		t.Errorf("license pre-flight should block install; got %d installed", len(res.Installed))
	}
}

func TestInstall_StrictModeMissingPlugin(t *testing.T) {
	// Mock registry has only one plugin; bundle expects 13. Strict mode → error.
	dir := setupOfflineRegistry(t, mockPartialNsentryRegistry())
	defer func() { _ = os.RemoveAll(dir) }()

	var buf bytes.Buffer
	_, err := Install(context.Background(), "nsentry", InstallOpts{
		DryRun:  true,
		Strict:  true,
		Out:     &buf,
		Channel: ChannelStable,
	})
	if err == nil {
		t.Fatal("expected strict-mode failure when plugins are missing")
	}
}

// --- helpers ---

// setupOfflineRegistry writes the given registry JSON to a temp file and
// points NSELF_PLUGIN_REGISTRY at the file:// URL so the registry fetcher
// resolves entirely offline.
//
// NOTE: file:// is not supported by the HTTPS-enforcing fetcher. Instead we
// pre-seed the cache directory with the registry JSON and let the fetcher
// pick it up via its stale-cache path. We set NSELF_PLUGIN_CACHE to a temp
// dir so the cache is isolated per-test.
func setupOfflineRegistry(t *testing.T, body []byte) string {
	t.Helper()
	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, "registry.json")
	if err := os.WriteFile(cachePath, body, 0600); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	// Set cache TTL via env to make the seeded cache appear fresh.
	t.Setenv("NSELF_PLUGIN_CACHE", cacheDir)
	// Point registry at localhost (which will fail) so we fall through to cache.
	// The cache path is checked first by Fetch().
	t.Setenv("NSELF_PLUGIN_REGISTRY", "http://127.0.0.1:1") // non-routable, forces fallback
	return cacheDir
}

// mockNsentryRegistry returns a minimal registry JSON containing every plugin
// in the nsentry bundle, all at version 1.0.0 and status "stable".
func mockNsentryRegistry() []byte {
	return []byte(`{
  "version": "test",
  "tier": "pro",
  "plugins": {
    "nself-uptime-monitor": {"name":"nself-uptime-monitor","version":"1.0.0","status":"stable","tier":"pro","requires_license":true},
    "nself-status-page":   {"name":"nself-status-page","version":"1.0.0","status":"stable","tier":"pro","requires_license":true},
    "nself-incident-mgmt": {"name":"nself-incident-mgmt","version":"1.0.0","status":"stable","tier":"pro","requires_license":true},
    "nself-alert-router":  {"name":"nself-alert-router","version":"1.0.0","status":"stable","tier":"pro","requires_license":true},
    "nself-slo-tracker":   {"name":"nself-slo-tracker","version":"1.0.0","status":"stable","tier":"pro","requires_license":true},
    "nself-synthetic-monitor":{"name":"nself-synthetic-monitor","version":"1.0.0","status":"stable","tier":"pro","requires_license":true},
    "nself-rum":           {"name":"nself-rum","version":"1.0.0","status":"stable","tier":"pro","requires_license":true},
    "nself-errors":        {"name":"nself-errors","version":"1.0.0","status":"stable","tier":"pro","requires_license":true},
    "nself-cron-monitor":  {"name":"nself-cron-monitor","version":"1.0.0","status":"stable","tier":"pro","requires_license":true},
    "nself-oncall":        {"name":"nself-oncall","version":"1.0.0","status":"stable","tier":"pro","requires_license":true},
    "nself-crash":         {"name":"nself-crash","version":"1.0.0","status":"stable","tier":"pro","requires_license":true},
    "nself-anomaly":       {"name":"nself-anomaly","version":"1.0.0","status":"stable","tier":"pro","requires_license":true},
    "nself-audit":         {"name":"nself-audit","version":"1.0.0","status":"stable","tier":"pro","requires_license":true}
  }
}`)
}

// mockPartialNsentryRegistry returns a registry with only one nsentry plugin
// to exercise strict-mode failure.
func mockPartialNsentryRegistry() []byte {
	return []byte(`{
  "version": "test",
  "tier": "pro",
  "plugins": {
    "nself-uptime-monitor": {"name":"nself-uptime-monitor","version":"1.0.0","status":"stable","tier":"pro","requires_license":true}
  }
}`)
}
