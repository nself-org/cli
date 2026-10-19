package bundle

// installer_appready_test.go — P7-PLUG-19: the app-ready flow of bundle install
// with fakes only (httptest /health, a fake installer, temp dirs; no network).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/bundle/appready"
	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// mockTaskRegistry serves the free Task Bundle's two plugins at 1.0.0.
func mockTaskRegistry() []byte {
	return []byte(`{"version":"test","tier":"free","plugins":{
    "notifications":{"name":"notifications","version":"1.0.0","status":"stable","tier":"free"},
    "jobs":{"name":"jobs","version":"1.0.0","status":"stable","tier":"free"}}}`)
}

// mockNotificationsOnly serves only notifications: jobs is skipped (non-strict).
func mockNotificationsOnly() []byte {
	return []byte(`{"version":"test","tier":"free","plugins":{
    "notifications":{"name":"notifications","version":"1.0.0","status":"stable","tier":"free"}}}`)
}

// healthServer answers /health with the migrations status applied of expected.
func healthServer(t *testing.T, applied, expected int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"status":"ok","migrations":{"applied":%d,"expected":%d}}`, applied, expected)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// writeMember writes <pluginDir>/<name>/plugin.json from the canonical v2 fixture
// with a boot-migrations declaration, one table and the given port.
func writeMember(t *testing.T, pluginDir, name string, port int, tables []string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "plugin", "manifestv2", "testdata", "v2", "full.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m["name"], m["version"], m["port"], m["tables"] = name, "1.0.0", port, tables
	m["schema"] = "np_" + strings.ReplaceAll(name, "-", "_")
	m["routes"] = []map[string]any{{"path": "/" + name, "upstream_port": port}}
	m["service"] = map[string]any{"compose": "docker-compose.plugin.yml", "healthcheck": "/health", "kind": "compose", "port": port}
	out, _ := json.Marshal(m)
	dir := filepath.Join(pluginDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func portOf(t *testing.T, srv *httptest.Server) int {
	t.Helper()
	var port int
	if _, err := fmt.Sscanf(srv.URL[strings.LastIndex(srv.URL, ":")+1:], "%d", &port); err != nil {
		t.Fatal(err)
	}
	return port
}

// freeOpts is a task bundle install with fakes; a bundle entitlement call would fail the test.
func freeOpts(t *testing.T, pluginDir string, out *bytes.Buffer, install func(name string) error) InstallOpts {
	t.Helper()
	return InstallOpts{
		Out: out, Channel: ChannelStable, PluginDir: pluginDir, Config: &config.Config{},
		bundleEntitledChecker: func(context.Context, string, string) (bool, error) {
			t.Error("ping_api entitlement was called for a free bundle")
			return false, nil
		},
		// A free bundle skips only the bundle entitlement call; the per-plugin
		// pre-flight still runs (and passes here).
		licenseChecker: func(context.Context, []string) error { return nil },
		installer:      func(_ context.Context, _ *config.Config, name, _ string) error { return install(name) },
		remover:        func(context.Context, *config.Config, string, string) error { return nil },
	}
}

func isolate(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	pluginDir := t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", pluginDir)
	return pluginDir
}

// TestBundleInstallDryRun: the dry run lists the nine steps in order and changes nothing.
func TestBundleInstallDryRun(t *testing.T) {
	pluginDir := isolate(t)
	setupOfflineRegistry(t, mockTaskRegistry())
	var out bytes.Buffer
	var hookCalls int32
	opts := freeOpts(t, pluginDir, &out, func(string) error { t.Error("dry run installed a plugin"); return nil })
	opts.DryRun = true
	count := func(context.Context) error { atomic.AddInt32(&hookCalls, 1); return nil }
	opts.Hooks = &appready.Hooks{Reconcile: count, Start: count}

	res, err := Install(opCtx(t), "task", opts)
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, out.String())
	}
	if res.Report != nil || len(res.Installed) != 0 {
		t.Fatalf("dry run result = %+v", res)
	}
	text := out.String()
	pos := -1
	for _, step := range appready.Order {
		i := strings.Index(text, " "+step+" ")
		if i < 0 || i <= pos {
			t.Fatalf("step %q missing or out of order (after %d, found %d) in:\n%s", step, pos, i, text)
		}
		pos = i
	}
	if want := "(dry-run) no changes made."; !strings.Contains(text, want) {
		t.Errorf("output lacks %q", want)
	}
	if !strings.Contains(text, "license: free bundle (no bundle entitlement call") {
		t.Errorf("free bundle plan should say it skips the bundle entitlement call:\n%s", text)
	}
	entries, _ := os.ReadDir(pluginDir)
	if len(entries) != 0 {
		t.Errorf("dry run touched the plugin dir: %v", entries)
	}
	if _, err := os.Stat(ledger.NewStore(pluginDir).Path()); !os.IsNotExist(err) {
		t.Errorf("dry run wrote the ledger: %v", err)
	}
	if hookCalls != 0 {
		t.Errorf("dry run called %d hooks", hookCalls)
	}
}

// TestBundleInstallMemberFailsE135: a member whose migrations never apply fails
// the readiness step. v1.5 returns E135 (exit 2) naming the plugin and the step;
// v1.4 prints the same report plus a warning and returns nil. Either way the
// ledger records exactly the installed members (jobs is not in the registry).
func TestBundleInstallMemberFailsE135(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		pluginDir := isolate(t)
		setupOfflineRegistry(t, mockNotificationsOnly())
		srv := healthServer(t, 0, 1)
		var out bytes.Buffer
		var reconciled, started int32
		opts := freeOpts(t, pluginDir, &out, func(name string) error {
			writeMember(t, pluginDir, name, portOf(t, srv), []string{"np_" + name + "_items"})
			return nil
		})
		opts.Hooks = &appready.Hooks{
			Reconcile: func(context.Context) error { atomic.AddInt32(&reconciled, 1); return nil },
			Timeout:   200 * time.Millisecond, Interval: 10 * time.Millisecond,
			ExportMetadata: func(context.Context) ([]byte, error) { t.Error("tracking ran after readiness failed"); return nil, nil },
		}
		// compat.V15(P7-PLUG-19): cmd builds the Start hook only in v1.5 (mirrored here).
		if compat.V15() {
			opts.Hooks.Start = func(context.Context) error { atomic.AddInt32(&started, 1); return nil }
		}

		res, err := Install(opCtx(t), "task", opts)
		text := out.String()
		if compat.V15() {
			for _, want := range []string{"NOT APP-READY: step readiness failed for plugin notifications", "readiness FAILED"} {
				if !strings.Contains(text, want) {
					t.Errorf("report lacks %q:\n%s", want, text)
				}
			}
			d := errs.Describe(err)
			if err == nil || d == nil || d.Code != "E135" || d.ExitCode != 2 {
				t.Fatalf("v1.5: err = %v, detail = %+v, want E135 exit 2", err, d)
			}
			for _, want := range []string{"notifications", "readiness"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("E135 message %q does not name %q", err.Error(), want)
				}
			}
			// The cause is the readiness E118.
			if cause := errs.Describe(errors.Unwrap(err)); cause == nil || cause.Code != "E118" {
				t.Errorf("E135 cause = %+v, want E118", cause)
			}
		} else {
			// v1.4: no start hook, so the four waiting steps are not verified
			// (skipped), nothing polls, the report is printed and the exit is 0.
			if err != nil {
				t.Fatalf("v1.4: err = %v, want nil", err)
			}
			for _, want := range []string{"NOT VERIFIED", "not verified (v1.5 behaviour, NSELF_V15=1)"} {
				if !strings.Contains(text, want) {
					t.Errorf("v1.4 report lacks %q:\n%s", want, text)
				}
			}
			for _, line := range strings.Split(text, "\n") {
				if strings.TrimSpace(line) == "APP-READY" {
					t.Errorf("v1.4 claims APP-READY:\n%s", text)
				}
			}
		}
		if res.Report == nil || res.Report.AppReady {
			t.Fatalf("report = %+v", res.Report)
		}
		wantStart := int32(0)
		wantFailed, wantFor := "", ""
		if compat.V15() {
			wantStart, wantFailed, wantFor = 1, "readiness", "notifications"
		}
		if res.Report.Failed != wantFailed || res.Report.FailedFor != wantFor {
			t.Errorf("report failed=%q for=%q, want %q/%q", res.Report.Failed, res.Report.FailedFor, wantFailed, wantFor)
		}
		if !res.Reconciled || reconciled != 1 || started != wantStart {
			t.Errorf("reconciled=%v reconcile calls=%d start calls=%d, want true/1/%d", res.Reconciled, reconciled, started, wantStart)
		}
		states := map[string]string{}
		for _, s := range res.Report.Steps {
			states[s.Name] = s.Result
		}
		wantStates := map[string]string{"licence": "ok", "fetch": "ok", "install": "ok", "ledger": "ok", "reconcile": "ok", "start": "ok", "readiness": "failed", "tracking": "skipped", "health": "skipped"}
		if !compat.V15() {
			wantStates["start"], wantStates["readiness"] = "skipped", "skipped"
		}
		for step, want := range wantStates {
			if states[step] != want {
				t.Errorf("step %s = %q, want %q", step, states[step], want)
			}
		}
		l, persisted, lerr := ledger.NewStore(pluginDir).Load()
		if lerr != nil || !persisted {
			t.Fatalf("ledger: persisted=%v err=%v", persisted, lerr)
		}
		if len(l.Plugins) != 1 || len(l.Bundles) != 1 || l.Plugins["notifications"].InstalledBy[0] != "task" {
			t.Errorf("ledger should record exactly notifications under task, got bundles=%v plugins=%v", l.Bundles, l.Plugins)
		}
	})
}

// TestBundleInstallAppReady: both members migrate, the tables are tracked and
// the services are healthy, so the install reports APP-READY and Reconciled.
func TestBundleInstallAppReady(t *testing.T) {
	pluginDir := isolate(t)
	setupOfflineRegistry(t, mockTaskRegistry())
	srv := healthServer(t, 1, 1)
	var out bytes.Buffer
	opts := freeOpts(t, pluginDir, &out, func(name string) error {
		writeMember(t, pluginDir, name, portOf(t, srv), []string{"np_" + name + "_items"})
		return nil
	})
	meta := `{"sources":[{"tables":[{"table":{"schema":"public","name":"np_notifications_items"}},{"table":{"name":"np_jobs_items"}}]}]}`
	opts.Hooks = &appready.Hooks{
		Reconcile:      func(context.Context) error { return nil },
		Start:          func(context.Context) error { return nil },
		ExportMetadata: func(context.Context) ([]byte, error) { return []byte(meta), nil },
		ServiceHealth:  func(context.Context) error { return nil },
		Timeout:        time.Second, Interval: 10 * time.Millisecond,
	}
	res, err := Install(opCtx(t), "task", opts)
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out.String())
	}
	if !res.Report.AppReady || !strings.Contains(out.String(), "APP-READY") || strings.Contains(out.String(), "NOT APP-READY") {
		t.Fatalf("not app-ready: %+v\n%s", res.Report, out.String())
	}
	l, _, _ := ledger.NewStore(pluginDir).Load()
	if len(l.Plugins) != 2 {
		t.Errorf("ledger plugins = %v, want notifications and jobs", l.Plugins)
	}
}

// TestBundleInstallLegacyNoHooks: nil Hooks records the ledger and runs no
// app-ready step; the caller reconciles (Reconciled stays false).
func TestBundleInstallLegacyNoHooks(t *testing.T) {
	pluginDir := isolate(t)
	setupOfflineRegistry(t, mockNotificationsOnly())
	srv := healthServer(t, 1, 1)
	var out bytes.Buffer
	opts := freeOpts(t, pluginDir, &out, func(name string) error {
		writeMember(t, pluginDir, name, portOf(t, srv), nil)
		return nil
	})
	res, err := Install(opCtx(t), "task", opts)
	if err != nil || res.Reconciled || !res.Changed || res.Report.AppReady {
		t.Fatalf("legacy: err=%v reconciled=%v changed=%v report=%+v", err, res.Reconciled, res.Changed, res.Report)
	}
	if strings.Contains(out.String(), "APP-READY") {
		t.Errorf("legacy path must not print a verdict:\n%s", out.String())
	}
	if l, persisted, _ := ledger.NewStore(pluginDir).Load(); !persisted || len(l.Plugins) != 1 {
		t.Errorf("ledger persisted=%v plugins=%v", persisted, l.Plugins)
	}
}

// TestBundleInstallNoProject: ErrNoProject from reconcile skips the rest, exits
// clean and is not called app-ready.
func TestBundleInstallNoProject(t *testing.T) {
	pluginDir := isolate(t)
	setupOfflineRegistry(t, mockNotificationsOnly())
	var out bytes.Buffer
	opts := freeOpts(t, pluginDir, &out, func(name string) error {
		writeMember(t, pluginDir, name, 1, nil)
		return nil
	})
	opts.Hooks = &appready.Hooks{Reconcile: func(context.Context) error { return appready.ErrNoProject },
		Start: func(context.Context) error { t.Error("start ran outside a project"); return nil }}
	res, err := Install(opCtx(t), "task", opts)
	if err != nil || res.Report.AppReady || !res.Reconciled || !strings.Contains(out.String(), "NOT VERIFIED") {
		t.Fatalf("no project: err=%v report=%+v\n%s", err, res.Report, out.String())
	}
}

// TestBundleInstallLedgerBootstrapsBeforeInstall: the ledger written before the
// member loop reflects the pre-install state, so a rollback leaves no new record.
func TestBundleInstallRollbackLeavesLedgerAtPreInstallState(t *testing.T) {
	pluginDir := isolate(t)
	setupOfflineRegistry(t, mockTaskRegistry())
	var out bytes.Buffer
	opts := freeOpts(t, pluginDir, &out, func(name string) error {
		if name == "jobs" {
			return errors.New("boom")
		}
		writeMember(t, pluginDir, name, 1, nil)
		return nil
	})
	opts.remover = func(_ context.Context, _ *config.Config, name, pd string) error {
		return os.RemoveAll(filepath.Join(pd, name))
	}
	if _, err := Install(opCtx(t), "task", opts); err == nil {
		t.Fatal("expected the member failure")
	}
	l, persisted, err := ledger.NewStore(pluginDir).Load()
	if err != nil || !persisted || len(l.Bundles) != 0 || len(l.Plugins) != 0 {
		t.Fatalf("ledger after rollback: persisted=%v err=%v bundles=%v plugins=%v", persisted, err, l.Bundles, l.Plugins)
	}
}
