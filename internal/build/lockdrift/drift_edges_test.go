package lockdrift

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/plugin"
)

func TestSourcesCatalogueAndPluginTiers(t *testing.T) {
	// LoadBytes only updates the in-memory catalogue; it never fetches or caches.
	if err := bundle.LoadBytes([]byte(`{"schema_version":"2.0","bundles":{
		"task":{"tier":"free","plugins":["basic"]},
		"chat":{"tier":"paid","plugins":["premium"]},
		"family":{"tier":"paid","plugins":[]}
	}}`)); err != nil {
		t.Fatal(err)
	}
	pluginDir := t.TempDir()
	cases := []struct {
		name            string
		tier            string
		requiresLicense bool
		licenseType     string
		wantTier        string
	}{
		{name: "basic", wantTier: ledger.TierFree},
		{name: "unknown", tier: "community", wantTier: ledger.TierFree},
		{name: "premium", tier: "pro", wantTier: ledger.TierLicensed},
		{name: "paid", tier: "paid", wantTier: ledger.TierLicensed},
		{name: "licensed", tier: "licensed", wantTier: ledger.TierLicensed},
		{name: "required", requiresLicense: true, wantTier: ledger.TierLicensed},
		{name: "license-type", licenseType: "pro", wantTier: ledger.TierLicensed},
	}
	wantPlugins := make(map[string]ledger.InstalledPlugin, len(cases))
	for _, tc := range cases {
		m := edgeManifest(tc.name)
		m.Tier, m.RequiresLicense, m.LicenseType = tc.tier, tc.requiresLicense, tc.licenseType
		writeEdgeManifest(t, pluginDir, m)
		wantPlugins[tc.name] = ledger.InstalledPlugin{
			Name: tc.name, Version: m.Version, Tier: tc.wantTier, Checksum: m.Checksum,
		}
	}

	src, err := Sources(pluginDir)()
	if err != nil {
		t.Fatalf("Sources: %v", err)
	}
	wantBundles := []ledger.BundleDef{
		{Slug: "task", Installable: true, Plugins: []string{"basic"}},
		{Slug: "chat", Installable: true, Plugins: []string{"premium"}},
		{Slug: "family", Installable: false, Plugins: []string{}},
		{Slug: "nself-plus", Installable: false, Plugins: []string{"premium"}},
	}
	if !reflect.DeepEqual(src.Bundles, wantBundles) {
		t.Fatalf("bundles = %#v, want %#v", src.Bundles, wantBundles)
	}
	gotPlugins := make(map[string]ledger.InstalledPlugin, len(src.Plugins))
	for _, p := range src.Plugins {
		gotPlugins[p.Name] = p
	}
	if len(src.Plugins) != len(cases) || !reflect.DeepEqual(gotPlugins, wantPlugins) {
		t.Fatalf("plugins = %#v, want %#v", src.Plugins, wantPlugins)
	}
}

func TestSourcesDirectoryReadError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "regular-file")
	if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := Sources(path)()
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) || pathErr.Path != path {
		t.Fatalf("Sources error = %v, want directory read error for %s", err, path)
	}
	if !reflect.DeepEqual(src, ledger.Sources{}) {
		t.Fatalf("failed Sources returned partial data: %#v", src)
	}
}

func TestPluginLedgerDriftWithoutPersistedLedger(t *testing.T) {
	for _, populated := range []bool{false, true} {
		name := "empty"
		if populated {
			name = "installed"
		}
		t.Run(name, func(t *testing.T) {
			pluginDir := t.TempDir()
			if populated {
				writeEdgeManifest(t, pluginDir, edgeManifest("alpha"))
			}
			drift, err := PluginLedgerDrift(pluginDir)
			if err != nil || drift != nil {
				t.Fatalf("unpersisted ledger: drift = %v, error = %v; want nil, nil", drift, err)
			}
			if _, err := os.Stat(filepath.Join(pluginDir, ledger.StateDirName)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("read-only bootstrap created state directory or stat failed: %v", err)
			}
		})
	}
}

func TestPluginLedgerDriftDamagedLedger(t *testing.T) {
	pluginDir := t.TempDir()
	path := filepath.Join(pluginDir, ledger.StateDirName, "bundles.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data := []byte("not JSON")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	drift, err := PluginLedgerDrift(pluginDir)
	if drift != nil || err == nil || !strings.HasPrefix(err.Error(), "reading the bundle ledger: ") ||
		!strings.Contains(err.Error(), "ledger: not a JSON object") || !strings.Contains(err.Error(), path) {
		t.Fatalf("damaged ledger: drift = %v, error = %v", drift, err)
	}
	if errors.Unwrap(err) == nil {
		t.Fatal("ledger error lost its wrapped cause")
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil || string(got) != string(data) {
		t.Fatalf("damaged ledger was modified: data = %q, error = %v", got, readErr)
	}
}

func TestPluginLedgerDriftChecksumOptional(t *testing.T) {
	checksum := strings.Repeat("a", 64)
	for _, tc := range []struct {
		name, recorded, actual string
	}{
		{name: "both absent"},
		{name: "ledger absent", actual: checksum},
		{name: "manifest absent", recorded: checksum},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pluginDir := t.TempDir()
			m := edgeManifest("alpha")
			m.Checksum = tc.actual
			m.Tier = "pro"
			writeEdgeManifest(t, pluginDir, m)
			writeEdgeLedger(t, pluginDir, map[string]ledger.PluginRecord{
				"alpha": {Version: m.Version, Tier: ledger.TierFree, Checksum: tc.recorded, Explicit: true},
			})
			// An unknown checksum on either side cannot prove drift. A tier
			// change also does not change the version/checksum comparison.
			drift, err := PluginLedgerDrift(pluginDir)
			if err != nil || len(drift) != 0 {
				t.Fatalf("optional checksum: drift = %v, error = %v", drift, err)
			}
		})
	}
}

func TestPluginLedgerDriftSortedDiagnostics(t *testing.T) {
	pluginDir := t.TempDir()
	for _, name := range []string{"zeta", "alpha", "extra"} {
		writeEdgeManifest(t, pluginDir, edgeManifest(name))
	}
	checksum := strings.Repeat("b", 64)
	writeEdgeLedger(t, pluginDir, map[string]ledger.PluginRecord{
		"zeta":    {Version: "0.9.0", Tier: ledger.TierFree, Checksum: checksum, Explicit: true},
		"alpha":   {Version: "0.9.0", Tier: ledger.TierFree, Checksum: checksum, Explicit: true},
		"missing": {Version: "1.0.0", Tier: ledger.TierFree, Explicit: true},
	})
	want := []string{
		"plugin alpha: ledger checksum " + checksum + " != dir checksum " + strings.Repeat("a", 64),
		"plugin alpha: ledger version 0.9.0 != dir version 1.0.0",
		"plugin extra: present in plugin dir but not in ledger",
		"plugin missing: ledger records version 1.0.0 but plugin dir has no manifest",
		"plugin zeta: ledger checksum " + checksum + " != dir checksum " + strings.Repeat("a", 64),
		"plugin zeta: ledger version 0.9.0 != dir version 1.0.0",
	}
	drift, err := PluginLedgerDrift(pluginDir)
	if err != nil || !reflect.DeepEqual(drift, want) {
		t.Fatalf("drift = %q, error = %v; want %q", drift, err, want)
	}
}

func edgeManifest(name string) plugin.PluginManifest {
	return plugin.PluginManifest{
		Name: name, Version: "1.0.0", Description: "test plugin", Category: "test", License: "MIT",
		Checksum: strings.Repeat("a", 64),
	}
}

func writeEdgeManifest(t *testing.T, pluginDir string, m plugin.PluginManifest) {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(pluginDir, m.Name, "plugin.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeEdgeLedger(t *testing.T, pluginDir string, plugins map[string]ledger.PluginRecord) {
	t.Helper()
	l := ledger.New()
	l.Plugins = plugins
	data, err := l.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(pluginDir, ledger.StateDirName, "bundles.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
