package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/bundle/appready"
	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
)

// writeLedgerFixturePlugin writes <dir>/<name>/plugin.json from the v2 fixture.
func writeLedgerFixturePlugin(t *testing.T, dir, name, version string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "internal", "plugin", "manifestv2", "testdata", "v2", "full.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m["name"], m["version"], m["schema"] = name, version, "np_"+strings.ReplaceAll(name, "-", "_")
	out, _ := json.Marshal(m)
	if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name, "plugin.json"), out, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestPluginInstallNeverBootstrapsLedger: no ledger file, none is created.
func TestPluginInstallNeverBootstrapsLedger(t *testing.T) {
	dir := t.TempDir()
	writeLedgerFixturePlugin(t, dir, "bots", "1.2.0")
	var w bytes.Buffer
	recordExplicitInstalls(&w, dir, []string{"bots"})
	if _, err := os.Stat(bundle.LedgerStore(dir).Path()); !os.IsNotExist(err) {
		t.Fatalf("a plugin install created the ledger: %v", err)
	}
	if w.Len() != 0 {
		t.Errorf("unexpected output %q", w.String())
	}
}

// TestPluginInstallRecordsExplicit: with a ledger present, an official plugin
// install stores explicit:true with the manifest version and tier; a plugin a
// bundle already recorded keeps its installed_by; a third-party URL is skipped.
func TestPluginInstallRecordsExplicit(t *testing.T) {
	dir := t.TempDir()
	store := bundle.LedgerStore(dir)
	if err := store.Update(func(l *ledger.Ledger) error {
		l.AddBundle("task", []ledger.InstalledPlugin{{Name: "jobs", Version: "1.0.0", Tier: ledger.TierFree}}, time.Now())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	writeLedgerFixturePlugin(t, dir, "bots", "1.2.0")
	writeLedgerFixturePlugin(t, dir, "jobs", "1.0.0")
	var w bytes.Buffer
	recordExplicitInstalls(&w, dir, []string{"bots", "jobs", "https://example.com/p.tar.gz"})
	l, _, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	b := l.Plugins["bots"]
	if !b.Explicit || b.Version != "1.2.0" || b.Tier != ledger.TierLicensed || len(b.InstalledBy) != 0 {
		t.Errorf("bots = %+v, want explicit, 1.2.0, licensed (fixture is tier pro), no bundle", b)
	}
	j := l.Plugins["jobs"]
	if !j.Explicit || len(j.InstalledBy) != 1 || j.InstalledBy[0] != "task" {
		t.Errorf("jobs = %+v, want explicit and still installed_by [task]", j)
	}
	if len(l.Plugins) != 2 {
		t.Errorf("plugins = %v, want only bots and jobs", l.Plugins)
	}
}

// TestPluginInstallLedgerFailureWarns: an unreadable ledger warns and does not panic or fail.
func TestPluginInstallLedgerFailureWarns(t *testing.T) {
	dir := t.TempDir()
	store := bundle.LedgerStore(dir)
	if err := os.MkdirAll(store.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path(), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeLedgerFixturePlugin(t, dir, "bots", "1.2.0")
	var w bytes.Buffer
	recordExplicitInstalls(&w, dir, []string{"bots"})
	if !strings.Contains(w.String(), "warning:") || !strings.Contains(w.String(), "bundle ledger") {
		t.Errorf("output = %q, want a ledger warning", w.String())
	}
}

// TestRemoveFreeBundleChangesNothing: `nself remove task` and `nself bundle
// remove task` are refused in both modes and leave the member plugins in place.
func TestRemoveFreeBundleChangesNothing(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("NSELF_PLUGIN_DIR", dir)
		t.Setenv("HOME", t.TempDir())
		writeLedgerFixturePlugin(t, dir, "notifications", "1.0.0") // before Chdir: the fixture path is relative
		writeLedgerFixturePlugin(t, dir, "jobs", "1.0.0")
		t.Chdir(t.TempDir())
		for name, run := range map[string]func() error{
			"nself remove task":        func() error { return runRemove(removeCmd, []string{"task"}) },
			"nself bundle remove task": func() error { return runBundleRemove(removeCmd, []string{"task"}) },
		} {
			err := run()
			if err == nil || !strings.Contains(err.Error(), "not installable/removable as a unit") {
				t.Errorf("%s: err = %v, want the free-bundle refusal", name, err)
			}
		}
		for _, p := range []string{"notifications", "jobs"} {
			if _, err := os.Stat(filepath.Join(dir, p, "plugin.json")); err != nil {
				t.Errorf("%s was removed: %v", p, err)
			}
		}
	})
}

// TestBundleHooksLoadConfigOnce: the metadata and health hooks poll; the project
// config loads once for the whole hook set, not on every poll.
func TestBundleHooksLoadConfigOnce(t *testing.T) {
	compattest.Set(t, true)
	t.Setenv("HOME", t.TempDir())
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, ".env"), []byte("PROJECT_NAME=hk\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj)
	oldLoad, oldMeta := bundleLoadConfig, bundleExportMetadata
	t.Cleanup(func() { bundleLoadConfig, bundleExportMetadata = oldLoad, oldMeta })
	loads := 0
	bundleLoadConfig = func(root string) (*config.Config, error) { loads++; return oldLoad(root) }
	bundleExportMetadata = func(context.Context, *config.Config) ([]byte, error) { return []byte(`{}`), nil }
	h := newBundleAppReadyHooks(bundleInstallCmd, "task")
	for i := 0; i < 5; i++ {
		if _, err := h.ExportMetadata(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if loads != 1 {
		t.Errorf("config loaded %d times over 5 polls, want 1", loads)
	}
}

// TestBundleReconcileHookOutsideProjectPrintsHint: the v1.4 hint is kept.
func TestBundleReconcileHookOutsideProjectPrintsHint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	var errBuf bytes.Buffer
	bundleInstallCmd.SetErr(&errBuf)
	t.Cleanup(func() { bundleInstallCmd.SetErr(nil) })
	h := newBundleAppReadyHooks(bundleInstallCmd, "task")
	if err := h.Reconcile(context.Background()); err != appready.ErrNoProject {
		t.Fatalf("err = %v, want ErrNoProject", err)
	}
	if !strings.Contains(errBuf.String(), "Extension files changed outside a project. Run 'nself build --plan'") {
		t.Errorf("stderr = %q, want the outside-project hint", errBuf.String())
	}
}
