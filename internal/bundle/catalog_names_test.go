package bundle

// Purpose: P7-PLUG-20 fix round 2: catalog member names are plain plugin slugs
// before any plan joins them to the plugin directory.
// Constraints: no network, no Docker.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/errs"
)

func TestLoadBytesRefusesPathShapedMemberNames(t *testing.T) {
	if err := LoadBytes([]byte(fixtureBundlesJSON)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = LoadBytes([]byte(fixtureBundlesJSON)) })
	for _, member := range []string{"../victim", "./x", "a/b", "..", "Mixed", ""} {
		doc := `{"schema_version":"2.0.0","bundles":{"evil":{"display":"E","tier":"free","plugins":["ok","` + member + `"]}}}`
		if err := LoadBytes([]byte(doc)); err == nil {
			t.Errorf("LoadBytes accepted member %q", member)
		}
		if _, ok := Get("evil"); ok {
			t.Errorf("a refused catalog (member %q) replaced the loaded one", member)
		}
	}
}

// TestComputePlanRefusesPathShapedMembers: a Bundle value carrying "../victim"
// must not be probed with os.Stat outside pluginDir.
func TestComputePlanRefusesPathShapedMembers(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "plugins")
	if err := os.MkdirAll(filepath.Join(root, "victim"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	b := Bundle{Slug: "evil", Plugins: []string{"../victim"}}
	// The name check runs before the ledger is consulted, so the error is the name's E401
	// and not "bundle is not recorded".
	_, err := computePlan(b, ledger.Ledger{}, pluginDir)
	if d := errs.Describe(err); d == nil || d.Code != "E401" {
		t.Fatalf("computePlan = %v, want E401 for ../victim", err)
	}
}

// TestComputePlanRefusesInvalidLedgerNames: a ledger record named "notify.prev"
// used to be probed and scheduled, so the removal failed mid-run after the other
// members were already gone. The whole plan must be refused up front.
func TestComputePlanRefusesInvalidLedgerNames(t *testing.T) {
	if err := LoadBytes([]byte(fixtureBundlesJSON)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = LoadBytes([]byte(fixtureBundlesJSON)) })
	for _, bad := range []string{"notify.prev", "bin", "common", "../x"} {
		pluginDir := t.TempDir()
		for _, n := range []string{"bots", bad} {
			if err := os.MkdirAll(filepath.Join(pluginDir, n), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		l := ledger.Ledger{
			SchemaVersion: ledger.SchemaVersion,
			Bundles:       map[string]ledger.BundleRecord{"chat": {}},
			Plugins: map[string]ledger.PluginRecord{
				"bots": {InstalledBy: []string{"chat"}},
				bad:    {InstalledBy: []string{"chat"}},
			},
		}
		b, _ := Get("chat")
		_, err := computePlan(b, l, pluginDir)
		if d := errs.Describe(err); d == nil || d.Code != "E401" {
			t.Errorf("computePlan with ledger name %q = %v, want E401 for the whole plan", bad, err)
		}
	}
}
