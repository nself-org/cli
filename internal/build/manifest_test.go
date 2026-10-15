package build

// Purpose: Repro + regression tests for PCI plugin-injection-dropped
//          (2026-07-03): plugins declared in nself.yaml must ALL be wired
//          into the generated stack, or loudly reported — never silently
//          dropped.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/config"
)

// fixtureBundlesJSON seeds internal/bundle's resolver so this package's
// tests are deterministic and offline — no network dependency on
// plugins.nself.org. Mirrors real bundles.json's "sentry" membership
// exactly (14 plugins, including nself-stripe per ADR-P6-03 Ruling 2) so
// TestLoadProjectManifest_FlatPluginsAndBundleExpansion's expected count
// stays honest against the real source of truth, not a stale hand copy.
const fixtureBundlesJSON = `{
  "schema_version": "2.0.0",
  "bundles": {
    "sentry": {"display": "ɳSentry", "tier": "paid", "price_monthly": 0.99, "price_yearly": 9.99, "plugins": ["nself-alert-router","nself-anomaly","nself-audit","nself-crash","nself-cron-monitor","nself-errors","nself-incident-mgmt","nself-oncall","nself-rum","nself-slo-tracker","nself-status-page","nself-stripe","nself-synthetic-monitor","nself-uptime-monitor"]}
  }
}`

func TestMain(m *testing.M) {
	if err := bundle.LoadBytes([]byte(fixtureBundlesJSON)); err != nil {
		panic("seeding bundle fixture: " + err.Error())
	}
	os.Exit(m.Run())
}

// writeTestPlugin scaffolds a fake installed plugin with a plugin.json and,
// when withCompose is true, a docker-compose.plugin.yml fragment.
func writeTestPlugin(t *testing.T, pluginDir, name string, port int, withCompose bool) {
	t.Helper()
	dir := filepath.Join(pluginDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"name":%q,"port":%d,"language":"go"}`, name, port)
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if withCompose {
		compose := fmt.Sprintf("services:\n  %s:\n    image: nself/%s:latest\n", name, name)
		if err := os.WriteFile(filepath.Join(dir, pluginComposeFilename), []byte(compose), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadProjectManifest_Absent(t *testing.T) {
	m, err := LoadProjectManifest(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m != nil {
		t.Fatalf("expected nil manifest for empty dir, got %+v", m)
	}
}

func TestLoadProjectManifest_TieredPlugins(t *testing.T) {
	dir := t.TempDir()
	yaml := `
app: ntask
bundle: ntask
plugins:
  free:
    - cron
    - notify
    - webhooks
  pro:
    - ai-gateway
`
	if err := os.WriteFile(filepath.Join(dir, "nself.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadProjectManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := m.DeclaredPlugins()
	want := []string{"ai-gateway", "cron", "notify", "webhooks"}
	if len(got) != len(want) {
		t.Fatalf("declared = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("declared = %v, want %v", got, want)
		}
	}
}

func TestLoadProjectManifest_FlatPluginsAndBundleExpansion(t *testing.T) {
	dir := t.TempDir()
	yaml := `
app: ops
bundles:
  - nsentry
plugins:
  - cron
`
	if err := os.WriteFile(filepath.Join(dir, "nself.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadProjectManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := m.DeclaredPlugins()
	// cron + the 14 sentry bundle plugins (bundles.json/ADR-P6-03 — includes
	// nself-stripe, which the pre-ADR hardcoded 13-plugin list omitted).
	if len(got) != 15 {
		t.Fatalf("expected 15 declared plugins (cron + 14 sentry), got %d: %v", len(got), got)
	}
	set := make(map[string]bool, len(got))
	for _, p := range got {
		set[p] = true
	}
	for _, want := range []string{"cron", "nself-uptime-monitor", "nself-status-page", "nself-errors", "nself-audit", "nself-stripe"} {
		if !set[want] {
			t.Errorf("declared plugins missing %q: %v", want, got)
		}
	}
}

func TestDeclaredPlugins_FiltersPlaceholders(t *testing.T) {
	m := &ProjectManifest{}
	m.Plugins.Flat = []string{"cron", "(free plugins only — see: nself plugin list)", "", "Notify"}
	got := m.DeclaredPlugins()
	want := []string{"cron", "notify"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("declared = %v, want %v", got, want)
	}
}

// TestResolveDeclaredPlugins_AllDeclaredPluginsWired is the PCI repro:
// declare N plugins in nself.yaml, install them, and assert all N compose
// fragments are discovered — none silently dropped.
func TestResolveDeclaredPlugins_AllDeclaredPluginsWired(t *testing.T) {
	t.Setenv("NSELF_AUTO_INSTALL_PLUGINS", "false")
	workdir := t.TempDir()
	pluginDir := t.TempDir()

	declared := []string{"cron", "notify", "webhooks", "audit-log", "feature-flags"}
	yaml := "app: repro\nplugins:\n  free:\n"
	for _, p := range declared {
		yaml += "    - " + p + "\n"
		writeTestPlugin(t, pluginDir, p, 3700, true)
	}
	if err := os.WriteFile(filepath.Join(workdir, "nself.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{ProjectName: "repro"}
	missing := ResolveDeclaredPlugins(context.Background(), cfg, workdir, pluginDir, expectedCoreServices(cfg))
	if len(missing) != 0 {
		t.Fatalf("expected no missing plugins, got %v", missing)
	}

	files, err := DiscoverPluginComposeFiles(workdir, pluginDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(declared) {
		t.Fatalf("expected %d plugin compose files, got %d: %v", len(declared), len(files), files)
	}
	found := make(map[string]bool)
	for _, f := range files {
		found[filepath.Base(filepath.Dir(f))] = true
	}
	for _, p := range declared {
		if !found[p] {
			t.Errorf("declared plugin %q dropped from generated stack (files: %v)", p, files)
		}
	}
}

// TestResolveDeclaredPlugins_MissingPluginReported asserts a declared but
// uninstalled (and un-installable) plugin is reported, not silently dropped.
func TestResolveDeclaredPlugins_MissingPluginReported(t *testing.T) {
	t.Setenv("NSELF_AUTO_INSTALL_PLUGINS", "false")
	workdir := t.TempDir()
	pluginDir := t.TempDir()

	writeTestPlugin(t, pluginDir, "cron", 3706, true)
	yaml := "app: repro\nplugins:\n  free:\n    - cron\n    - search\n    - jobs\n"
	if err := os.WriteFile(filepath.Join(workdir, "nself.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{ProjectName: "repro"}
	missing := ResolveDeclaredPlugins(context.Background(), cfg, workdir, pluginDir, expectedCoreServices(cfg))
	if len(missing) != 2 {
		t.Fatalf("expected 2 missing plugins (search, jobs), got %v", missing)
	}
	set := map[string]bool{missing[0]: true, missing[1]: true}
	if !set["search"] || !set["jobs"] {
		t.Fatalf("missing = %v, want [jobs search]", missing)
	}
}

// TestResolveDeclaredPlugins_CoreServiceSatisfies asserts the declared name
// auth, which hasura-auth provides, does not produce a false missing-plugin
// report.
func TestResolveDeclaredPlugins_CoreServiceSatisfies(t *testing.T) {
	t.Setenv("NSELF_AUTO_INSTALL_PLUGINS", "false")
	workdir := t.TempDir()
	pluginDir := t.TempDir()

	yaml := "app: repro\nplugins:\n  free:\n    - auth\n"
	if err := os.WriteFile(filepath.Join(workdir, "nself.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{ProjectName: "repro"}
	missing := ResolveDeclaredPlugins(context.Background(), cfg, workdir, pluginDir, expectedCoreServices(cfg))
	if len(missing) != 0 {
		t.Fatalf("auth is a core service — expected no missing plugins, got %v", missing)
	}
}

// TestResolveDeclaredPlugins_StoragePluginNotCoveredByMinio guards D-0451: the
// free storage plugin is a plugin, so MinIO being enabled must not count as
// covering a declared `storage` (directly or through a bundle).
func TestResolveDeclaredPlugins_StoragePluginNotCoveredByMinio(t *testing.T) {
	t.Setenv("NSELF_AUTO_INSTALL_PLUGINS", "false")
	workdir := t.TempDir()
	pluginDir := t.TempDir()

	yaml := "app: repro\nplugins:\n  free:\n    - storage\n"
	if err := os.WriteFile(filepath.Join(workdir, "nself.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{ProjectName: "repro"}
	cfg.Minio.Enabled = true
	missing := ResolveDeclaredPlugins(context.Background(), cfg, workdir, pluginDir, expectedCoreServices(cfg))
	if len(missing) != 1 || missing[0] != "storage" {
		t.Fatalf("storage plugin is not provided by MinIO — want missing [storage], got %v", missing)
	}

	writeTestPlugin(t, pluginDir, "storage", 9007, true)
	if missing := ResolveDeclaredPlugins(context.Background(), cfg, workdir, pluginDir, expectedCoreServices(cfg)); len(missing) != 0 {
		t.Fatalf("installed storage plugin must satisfy the declaration, got missing %v", missing)
	}
}

// TestResolveDeclaredPlugins_NoManifestNoOp asserts projects without
// nself.yaml keep the legacy install-then-discover flow untouched.
func TestResolveDeclaredPlugins_NoManifestNoOp(t *testing.T) {
	cfg := &config.Config{ProjectName: "legacy"}
	missing := ResolveDeclaredPlugins(context.Background(), cfg, t.TempDir(), t.TempDir(), expectedCoreServices(cfg))
	if missing != nil {
		t.Fatalf("expected nil missing for manifest-less project, got %v", missing)
	}
}

func TestDefaultPluginDir_EnvOverride(t *testing.T) {
	t.Setenv("NSELF_PLUGIN_DIR", "/tmp/custom-plugins")
	if got := DefaultPluginDir(); got != "/tmp/custom-plugins" {
		t.Fatalf("DefaultPluginDir() = %q, want /tmp/custom-plugins", got)
	}
}
