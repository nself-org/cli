package reconcile

// Purpose: tests for the removed-plugin prune of the generated-state record
// (debt D-0557).
// Inputs: synthetic records and a temp NSELF_PLUGIN_DIR.
// Outputs: pass/fail on which "@plugins/" keys survive PlannedRecord.
// Constraints: temp dirs only; no network.

import (
	"os"
	"path/filepath"
	"testing"

	nbuild "github.com/nself-org/cli/internal/build"
)

// TestPruneGonePluginsKeepsLiveAndPlanned: only "@plugins/" keys whose plugin
// entry is gone are dropped; project keys, live plugins and keys the build
// writes this run stay.
func TestPruneGonePluginsKeepsLiveAndPlanned(t *testing.T) {
	s := &GeneratedState{Files: map[string]string{
		"docker-compose.yml":                                "p",
		PluginsPrefix + "live/docker-compose.plugin.yml":    "l",
		PluginsPrefix + "gone/docker-compose.plugin.yml":    "g",
		PluginsPrefix + "gone/extra.yml":                    "g2",
		PluginsPrefix + "planned/docker-compose.plugin.yml": "w",
		PluginsPrefix + "flat.yml":                          "f",
	}}
	keep := map[string]bool{PluginsPrefix + "planned/docker-compose.plugin.yml": true}
	present := map[string]bool{"live": true, "flat.yml": true}
	s.pruneGonePlugins(keep, func(entry string) bool { return present[entry] })

	want := []string{"docker-compose.yml", PluginsPrefix + "live/docker-compose.plugin.yml",
		PluginsPrefix + "planned/docker-compose.plugin.yml", PluginsPrefix + "flat.yml"}
	for _, k := range want {
		if _, ok := s.Files[k]; !ok {
			t.Errorf("%s must stay in the record", k)
		}
	}
	for _, k := range []string{PluginsPrefix + "gone/docker-compose.plugin.yml", PluginsPrefix + "gone/extra.yml"} {
		if _, ok := s.Files[k]; ok {
			t.Errorf("%s belongs to a removed plugin and must leave the record", k)
		}
	}
}

// TestPlannedRecordDropsRemovedPlugin: through PlannedRecord, a removed
// plugin's fragment key leaves the record while an installed plugin's key and
// a fragment the build writes this run stay. Folding twice gives the same
// bytes, so a plan after the apply does not list the record again.
func TestPlannedRecordDropsRemovedPlugin(t *testing.T) {
	isolateEnv(t)
	plugins := t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", plugins)
	for _, d := range []string{"kept", "written"} {
		if err := os.MkdirAll(filepath.Join(plugins, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	keptKey := PluginsPrefix + "kept/docker-compose.plugin.yml"
	goneKey := PluginsPrefix + "removed/docker-compose.plugin.yml"
	writtenKey := PluginsPrefix + "written/docker-compose.plugin.yml"
	cur := &GeneratedState{Files: map[string]string{keptKey: "k", goneKey: "r", "docker-compose.yml": "c"}}
	pb := &nbuild.PlannedBuild{Files: map[string]nbuild.PlannedFile{
		filepath.Join(plugins, "written", "docker-compose.plugin.yml"): {Data: []byte("w\n")},
	}}

	next, body, err := PlannedRecord(cur, pb)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := next.Files[goneKey]; ok {
		t.Fatalf("a removed plugin's key must leave the record: %v", next.Files)
	}
	for _, k := range []string{keptKey, writtenKey, "docker-compose.yml"} {
		if _, ok := next.Files[k]; !ok {
			t.Fatalf("%s must stay in the record: %v", k, next.Files)
		}
	}
	if _, ok := cur.Files[goneKey]; !ok {
		t.Fatal("PlannedRecord must not mutate the current record")
	}
	_, again, err := PlannedRecord(next, pb)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(body) {
		t.Fatalf("folding twice must be idempotent:\n%s\nvs\n%s", body, again)
	}
}
