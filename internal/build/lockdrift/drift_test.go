package lockdrift

import (
	"github.com/nself-org/cli/internal/bundle/ledger"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPluginLedgerDrift detects when the plugin dir has been updated (by copy
// or git pull) without re-running nself build: the ledger still records the
// old version/checksum, and --frozen must refuse to start (D-0343, P7-PLUG-19).
func TestPluginLedgerDrift(t *testing.T) {
	pluginDir := t.TempDir()
	stateDir := filepath.Join(pluginDir, ledger.StateDirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a ledger that records plugin "alpha" at v1.0.0 with a checksum.
	led := ledger.New()
	led.Plugins["alpha"] = ledger.PluginRecord{
		Version:  "1.0.0",
		Checksum: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Tier:     ledger.TierFree,
		Explicit: true,
	}
	raw, err := led.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "bundles.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	// No manifests on disk yet: drift reports the missing manifest.
	drift, err := PluginLedgerDrift(pluginDir)
	if err != nil {
		t.Fatalf("PluginLedgerDrift: %v", err)
	}
	if len(drift) == 0 {
		t.Fatal("expected drift when plugin dir has no manifest for a ledger entry")
	}
	found := false
	for _, d := range drift {
		if strings.Contains(d, "alpha") && strings.Contains(d, "no manifest") {
			found = true
		}
	}
	if !found {
		t.Fatalf("drift did not name alpha as missing: %v", drift)
	}

	// Now write a manifest with a DIFFERENT version: version drift.
	alphaDir := filepath.Join(pluginDir, "alpha")
	if err := os.MkdirAll(alphaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"alpha","version":"2.0.0","description":"d","category":"c","license":"MIT","checksum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`
	if err := os.WriteFile(filepath.Join(alphaDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	drift, err = PluginLedgerDrift(pluginDir)
	if err != nil {
		t.Fatalf("PluginLedgerDrift: %v", err)
	}
	versionDrift := false
	for _, d := range drift {
		if strings.Contains(d, "alpha") && strings.Contains(d, "ledger version") && strings.Contains(d, "dir version") {
			versionDrift = true
		}
	}
	if !versionDrift {
		t.Fatalf("expected version drift, got: %v", drift)
	}

	// Fix the version but change the checksum: checksum drift.
	manifest2 := `{"name":"alpha","version":"1.0.0","description":"d","category":"c","license":"MIT","checksum":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`
	if err := os.WriteFile(filepath.Join(alphaDir, "plugin.json"), []byte(manifest2), 0o644); err != nil {
		t.Fatal(err)
	}
	drift, err = PluginLedgerDrift(pluginDir)
	if err != nil {
		t.Fatalf("PluginLedgerDrift: %v", err)
	}
	checksumDrift := false
	for _, d := range drift {
		if strings.Contains(d, "alpha") && strings.Contains(d, "ledger checksum") && strings.Contains(d, "dir checksum") {
			checksumDrift = true
		}
	}
	if !checksumDrift {
		t.Fatalf("expected checksum drift, got: %v", drift)
	}

	// Matching manifest: no drift.
	manifest3 := `{"name":"alpha","version":"1.0.0","description":"d","category":"c","license":"MIT","checksum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`
	if err := os.WriteFile(filepath.Join(alphaDir, "plugin.json"), []byte(manifest3), 0o644); err != nil {
		t.Fatal(err)
	}
	drift, err = PluginLedgerDrift(pluginDir)
	if err != nil {
		t.Fatalf("PluginLedgerDrift: %v", err)
	}
	if len(drift) != 0 {
		t.Fatalf("expected no drift for matching manifest, got: %v", drift)
	}
}
