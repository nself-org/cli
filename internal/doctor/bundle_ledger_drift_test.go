package doctor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLedger(t *testing.T, pluginDir, body string) {
	t.Helper()
	dir := filepath.Join(pluginDir, ".nself-state")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bundles.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func installPlain(t *testing.T, pluginDir, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(pluginDir, name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, name, "plugin.json"), []byte(`{"name":"`+name+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

const driftLedger = `{"_generated":"x","schema_version":1,"bundles":{"task":{"installed_at":"2026-10-10T00:00:00Z"}},
"plugins":{"notifications":{"installed_by":["task"],"explicit":false},"jobs":{"installed_by":["task"],"explicit":false}}}`

// TestLedgerDriftE136: the ledger lists a plugin whose directory is gone.
func TestLedgerDriftE136(t *testing.T) {
	dir := t.TempDir()
	writeLedger(t, dir, driftLedger)
	installPlain(t, dir, "notifications") // jobs is missing
	res := BundleLedgerDriftChecks(context.Background(), dir)
	if len(res) != 1 {
		t.Fatalf("results = %+v, want exactly one", res)
	}
	r := res[0]
	if r.Status != "fail" || r.Section != "plugins" || r.Name != "BUNDLE-LEDGER-DRIFT-jobs" ||
		!strings.HasPrefix(r.Message, "E136:") || !strings.Contains(r.Message, "jobs") || !strings.Contains(r.FixCmd, "nself bundle install") {
		t.Errorf("result = %+v", r)
	}
	// A plugin directory without plugin.json counts as gone too.
	if err := os.Remove(filepath.Join(dir, "notifications", "plugin.json")); err != nil {
		t.Fatal(err)
	}
	if res := BundleLedgerDriftChecks(context.Background(), dir); len(res) != 2 {
		t.Errorf("both plugins gone: %+v", res)
	}
}

// TestLedgerDriftConsistentSilent: no ledger, or a ledger matching the disk, gives nothing.
func TestLedgerDriftConsistentSilent(t *testing.T) {
	dir := t.TempDir()
	if res := BundleLedgerDriftChecks(context.Background(), dir); len(res) != 0 {
		t.Fatalf("no ledger: %+v", res)
	}
	if res := BundleLedgerDriftChecks(context.Background(), filepath.Join(dir, "missing")); len(res) != 0 {
		t.Fatalf("no plugin dir: %+v", res)
	}
	writeLedger(t, dir, driftLedger)
	installPlain(t, dir, "notifications")
	installPlain(t, dir, "jobs")
	if res := BundleLedgerDriftChecks(context.Background(), dir); len(res) != 0 {
		t.Fatalf("consistent ledger: %+v", res)
	}
	if res := BundleLedgerDriftChecks(context.Background(), ""); len(res) != 0 {
		t.Fatalf("empty dir: %+v", res)
	}
}

func TestLedgerDriftDamagedAndHostile(t *testing.T) {
	dir := t.TempDir()
	writeLedger(t, dir, `{not json`)
	res := BundleLedgerDriftChecks(context.Background(), dir)
	if len(res) != 1 || res[0].Status != "warn" || !strings.Contains(res[0].Message, "bundles.json") {
		t.Fatalf("damaged ledger: %+v", res)
	}
	writeLedger(t, dir, `{"plugins":{"../escape":{},"a/b":{}}}`)
	if res := BundleLedgerDriftChecks(context.Background(), dir); len(res) != 0 {
		t.Fatalf("path-like slugs must be ignored, got %+v", res)
	}
}

func TestBundlePluginDirResolution(t *testing.T) {
	t.Setenv("NSELF_PLUGIN_DIR", "/x/plugins")
	if got := bundlePluginDir(); got != "/x/plugins" {
		t.Errorf("env first: %s", got)
	}
	t.Setenv("NSELF_PLUGIN_DIR", "")
	t.Setenv("HOME", "/h")
	t.Setenv("USERPROFILE", "/h")
	if got := bundlePluginDir(); got != filepath.Join("/h", ".nself", "plugins") {
		t.Errorf("home fallback: %s", got)
	}
}
