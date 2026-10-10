package bundle

// installer_appready_fix_test.go — P7-PLUG-19 fix round 1: reconcile failure in
// both modes, the free-bundle licence pre-flight and the ledger dir next to
// the plugins.

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle/appready"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin"
)

// TestBundleInstallReconcileFailureReturnsError: a failing Reconcile hook
// returns its error in both modes (P7-LIVE-06 behaviour), and it is not E135.
func TestBundleInstallReconcileFailureReturnsError(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		pluginDir := isolate(t)
		setupOfflineRegistry(t, mockNotificationsOnly())
		var out bytes.Buffer
		opts := freeOpts(t, pluginDir, &out, func(name string) error {
			writeMember(t, pluginDir, name, 1, nil)
			return nil
		})
		boom := errors.New("regenerate failed")
		opts.Hooks = &appready.Hooks{Reconcile: func(context.Context) error { return boom }}
		res, err := Install(context.Background(), "task", opts)
		if err == nil || !errors.Is(err, boom) {
			t.Fatalf("err = %v, want it to wrap the reconcile error", err)
		}
		if d := errs.Describe(err); d != nil && d.Code == "E135" {
			t.Errorf("a reconcile failure must not be E135: %+v", d)
		}
		if res.Report == nil || res.Report.Failed != "reconcile" || !res.Reconciled {
			t.Errorf("report = %+v reconciled = %v", res.Report, res.Reconciled)
		}
	})
}

// TestFreeBundleStillRunsPluginLicenceCheck: IsFree is a price heuristic. A
// "FREE" bundle whose member needs a licence, with no key anywhere, fails the
// licence step through the per-plugin pre-flight; the bundle entitlement call
// (Phase 1) is never made.
func TestFreeBundleStillRunsPluginLicenceCheck(t *testing.T) {
	pluginDir := isolate(t)
	for _, k := range []string{"NSELF_PLUGIN_LICENSE_KEY", "NSELF_PLUGIN_LICENSE_KEY_OWNER"} {
		t.Setenv(k, "")
	}
	for _, k := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"} {
		t.Setenv("NSELF_LICENSE_KEY_"+k, "")
	}
	setupOfflineRegistry(t, []byte(`{"version":"test","tier":"pro","plugins":{
    "notifications":{"name":"notifications","version":"1.0.0","status":"stable","tier":"pro","requires_license":true},
    "jobs":{"name":"jobs","version":"1.0.0","status":"stable","tier":"pro","requires_license":true}}}`))
	if b, _ := Get("task"); !b.IsFree() {
		t.Fatal("fixture assumption: the task bundle is free by price")
	}
	var out bytes.Buffer
	opts := freeOpts(t, pluginDir, &out, func(string) error { t.Error("a plugin was installed after the licence step failed"); return nil })
	opts.licenseChecker = nil // the real per-plugin pre-flight
	res, err := Install(context.Background(), "task", opts)
	if err == nil || !strings.Contains(err.Error(), "license validation failed") {
		t.Fatalf("err = %v, want license validation failed", err)
	}
	if res.Report == nil || res.Report.Failed != "licence" {
		t.Errorf("report = %+v, want the licence step failed", res.Report)
	}
}

// TestLedgerDirIsNotAPlugin: the .nself-state dir next to the plugins is never
// listed as an installed plugin.
func TestLedgerDirIsNotAPlugin(t *testing.T) {
	pluginDir := isolate(t)
	writeMember(t, pluginDir, "notifications", 1, nil)
	state := filepath.Join(pluginDir, ".nself-state")
	if err := os.MkdirAll(state, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "bundles.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	list, err := plugin.ListInstalled(pluginDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "notifications" {
		t.Fatalf("ListInstalled = %+v, want only notifications", list)
	}
}
