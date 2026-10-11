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
