package bundle

// installer_appready_debt_test.go — D-0372 (a skipped member is never
// APP-READY), D-0338 (installed_by names every recorded bundle the catalogue
// lists the member under) and D-0374 (the strict full-catalogue bootstrap
// source). Temp dirs and fakes only, no network.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/bundle/appready"
	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/config"
)

// TestBundleInstallSkippedMemberNotAppReady: jobs is missing from the registry
// (non-strict skip); every app-ready step passes for notifications, yet the run
// is NOT VERIFIED and names jobs.
func TestBundleInstallSkippedMemberNotAppReady(t *testing.T) {
	guardOperationLockTest(t)
	pluginDir := isolate(t)
	setupOfflineRegistry(t, mockNotificationsOnly())
	srv := healthServer(t, 1, 1)
	var out bytes.Buffer
	opts := freeOpts(t, pluginDir, &out, func(name string) error {
		writeMember(t, pluginDir, name, portOf(t, srv), []string{"np_" + name + "_items"})
		return nil
	})
	meta := `{"sources":[{"tables":[{"table":{"schema":"public","name":"np_notifications_items"}}]}]}`
	opts.Hooks = &appready.Hooks{
		Reconcile:      func(context.Context) error { return nil },
		Start:          func(context.Context) error { return nil },
		ExportMetadata: func(context.Context) ([]byte, error) { return []byte(meta), nil },
		ServiceHealth:  func(context.Context) error { return nil },
		Timeout:        time.Second, Interval: 10 * time.Millisecond,
	}
	res, err := Install(context.Background(), "task", opts)
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out.String())
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != "jobs" {
		t.Fatalf("setup: skipped = %v, want [jobs]", res.Skipped)
	}
	if res.Report.AppReady || !strings.Contains(res.Report.Note, "jobs") {
		t.Fatalf("report = %+v, want not app-ready with a note naming jobs", res.Report)
	}
	if !strings.Contains(out.String(), "NOT VERIFIED") || strings.Contains(out.String(), "\nAPP-READY") {
		t.Errorf("output does not say NOT VERIFIED:\n%s", out.String())
	}
}

// paidOpts is an overlap-catalogue install with fakes and a granted entitlement.
func paidOpts(pluginDir string, out *bytes.Buffer, install func(string) error) InstallOpts {
	return InstallOpts{
		Out: out, Channel: ChannelStable, PluginDir: pluginDir, Config: &config.Config{},
		bundleEntitledChecker: func(context.Context, string, string) (bool, error) { return true, nil },
		licenseChecker:        func(context.Context, []string) error { return nil },
		installer:             func(_ context.Context, _ *config.Config, name, _ string) error { return install(name) },
		remover:               func(context.Context, *config.Config, string, string) error { return nil },
	}
}

// TestBundleInstallRestoresCatalogHolders: chat and claw are recorded and share
// py. A forced remove of py (ForgetPlugin + delete) and a repair `bundle install
// chat` must leave py installed_by [chat claw], not [chat]. Unrecorded family
// is never added; px (chat only) stays [chat].
func TestBundleInstallRestoresCatalogHolders(t *testing.T) {
	pluginDir := isolate(t)
	useOverlapBundles(t)
	setupOfflineRegistry(t, []byte(`{"version":"test","tier":"free","plugins":{
    "px":{"name":"px","version":"1.0.0","status":"stable","tier":"free"},
    "py":{"name":"py","version":"1.0.0","status":"stable","tier":"free"}}}`))
	installPlugins(t, pluginDir, "px", "py", "pz")
	recordBundle(t, pluginDir, "chat")
	recordBundle(t, pluginDir, "claw")
	clawAt := loadLedger(t, pluginDir).Bundles["claw"].InstalledAt
	if err := ForgetPlugin(pluginDir, "py"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(pluginDir, "py")); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	opts := paidOpts(pluginDir, &out, func(name string) error { writeMember(t, pluginDir, name, 1, nil); return nil })
	if _, err := Install(context.Background(), "chat", opts); err != nil {
		t.Fatalf("install chat: %v\n%s", err, out.String())
	}
	l := loadLedger(t, pluginDir)
	if by := l.Plugins["py"].InstalledBy; !sameStrings(by, []string{"chat", "claw"}) || l.Plugins["py"].Explicit {
		t.Errorf("py = %+v, want installed_by [chat claw], not explicit", l.Plugins["py"])
	}
	if by := l.Plugins["px"].InstalledBy; !sameStrings(by, []string{"chat"}) {
		t.Errorf("px installed_by = %v, want [chat]", by)
	}
	if _, ok := l.Bundles["family"]; ok {
		t.Error("an unrecorded bundle was recorded")
	}
	if got := l.Bundles["claw"].InstalledAt; got != clawAt {
		t.Errorf("claw installed_at changed: %s -> %s", clawAt, got)
	}
}

// TestAddCatalogHoldersKeepsExistingReferences: a reference the catalogue no
// longer backs is kept (errs toward keeping), and the installing bundle is not
// re-added through the catalogue loop.
func TestAddCatalogHoldersKeepsExistingReferences(t *testing.T) {
	useOverlapBundles(t)
	l := ledger.New()
	at := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	l.AddBundle("claw", []ledger.InstalledPlugin{{Name: "pz", Version: "1.0.0"}}, at)
	l.AddBundle("family", []ledger.InstalledPlugin{{Name: "px", Version: "1.0.0"}}, at) // stale: family does not list px
	l.AddBundle("chat", []ledger.InstalledPlugin{{Name: "px", Version: "1.0.0"}, {Name: "py", Version: "1.0.0"}}, at)
	addCatalogHolders(&l, "chat", []ledger.InstalledPlugin{{Name: "px"}, {Name: "py"}}, at.Add(time.Hour))
	if by := l.Plugins["py"].InstalledBy; !sameStrings(by, []string{"chat", "claw"}) {
		t.Errorf("py installed_by = %v, want [chat claw]", by)
	}
	if by := l.Plugins["px"].InstalledBy; !sameStrings(by, []string{"chat", "family"}) {
		t.Errorf("px installed_by = %v, want the stale family reference kept", by)
	}
	if v := l.Plugins["py"].Version; v != "1.0.0" {
		t.Errorf("py version = %q: a name-only add must keep recorded facts", v)
	}
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
}

// TestBootstrapSourcesFullCatalogue: a catalogue bundle outside DisplayOrder is
// recorded by bootstrap; LedgerSources (display order) would treat its member
// as explicit.
func TestBootstrapSourcesFullCatalogue(t *testing.T) {
	isolate(t)
	if err := LoadBytes([]byte(`{"schema_version":"2.0.0","bundles":{
    "task":{"display":"Task","tier":"free","price_monthly":0,"price_yearly":0,"plugins":["pg"]},
    "extra":{"display":"Extra","tier":"paid","price_monthly":1,"price_yearly":10,"plugins":["pe"]}}}`)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = LoadBytes([]byte(fixtureBundlesJSON)) })
	dir := t.TempDir()
	installPlugins(t, dir, "pe")
	st := newLedgerStore(dir)
	if _, err := st.EnsureBootstrapped(); err != nil {
		t.Fatal(err)
	}
	l := loadLedger(t, dir)
	if _, ok := l.Bundles["extra"]; !ok || l.Plugins["pe"].Explicit {
		t.Errorf("ledger = %+v, want extra recorded and pe bundle-owned", l)
	}
}

// TestBootstrapSourcesUnreadableManifest: a plugin.json that exists but does not
// parse blocks a persisted bootstrap and is named; a directory without one is
// not a plugin and is ignored.
func TestBootstrapSourcesUnreadableManifest(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	installPlugins(t, dir, "notifications")
	for name, body := range map[string]string{"broken": "{not json", "empty-dir": ""} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if body != "" {
			if err := os.WriteFile(filepath.Join(dir, name, "plugin.json"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	src, err := bootstrapSources(dir)()
	if err != nil {
		t.Fatal(err)
	}
	if len(src.Unreadable) != 1 || !strings.HasPrefix(src.Unreadable[0], "broken: ") {
		t.Errorf("unreadable = %q, want only broken", src.Unreadable)
	}
	if _, err := newLedgerStore(dir).EnsureBootstrapped(); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Errorf("EnsureBootstrapped = %v, want a refusal naming broken", err)
	}
	if _, err := os.Stat(newLedgerStore(dir).Path()); !os.IsNotExist(err) {
		t.Errorf("a ledger was persisted from an incomplete scan: %v", err)
	}
}
