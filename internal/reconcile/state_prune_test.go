package reconcile

// Purpose: tests for the removed-plugin prune of the generated-state record
// (debt D-0557).
// Inputs: synthetic records, a temp NSELF_PLUGIN_DIR and the dev-plugin
// fixture with the plugin directory set only in the project's .env (D-0607).
// Outputs: pass/fail on which "@plugins/" keys survive PlannedRecord.
// Constraints: temp dirs only; no network.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	nbuild "github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compat/compattest"
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

// shellPluginDirs are the process-env states D-0607 covers: NSELF_PLUGIN_DIR
// set only in the project's .env, the shell either unset or pointing elsewhere.
var shellPluginDirs = []struct {
	name  string
	shell func(t *testing.T) // puts the shell's NSELF_PLUGIN_DIR in place
}{
	{"shell unset", func(t *testing.T) { t.Helper(); _ = os.Unsetenv("NSELF_PLUGIN_DIR") }},
	{"shell different", func(t *testing.T) { t.Helper(); _ = os.Setenv("NSELF_PLUGIN_DIR", t.TempDir()) }},
}

// TestPlannedRecordPrunesUnderBuildPluginDir (D-0607): the prune judges a
// plugin entry under the plugin directory the build resolved (PlannedBuild),
// not the process env: an installed plugin the build did not rewrite keeps
// its key, a removed one still leaves.
func TestPlannedRecordPrunesUnderBuildPluginDir(t *testing.T) {
	for _, tc := range shellPluginDirs {
		t.Run(tc.name, func(t *testing.T) {
			isolateEnv(t)
			tc.shell(t)
			plugins := t.TempDir()
			if err := os.MkdirAll(filepath.Join(plugins, "installed"), 0o755); err != nil {
				t.Fatal(err)
			}
			keptKey := PluginsPrefix + "installed/docker-compose.plugin.yml"
			goneKey := PluginsPrefix + "removed/docker-compose.plugin.yml"
			cur := &GeneratedState{Files: map[string]string{keptKey: "k", goneKey: "r"}}
			next, _, err := PlannedRecord(cur, &nbuild.PlannedBuild{Files: map[string]nbuild.PlannedFile{}, PluginDir: plugins})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := next.Files[keptKey]; !ok {
				t.Fatalf("an installed plugin the build did not rewrite must keep its key: %v", next.Files)
			}
			if _, ok := next.Files[goneKey]; ok {
				t.Fatalf("a removed plugin's key must leave the record: %v", next.Files)
			}
		})
	}
}

// TestPluginDirOnlyInProjectEnv (D-0607) runs the real plan and apply with
// NSELF_PLUGIN_DIR set only in the project's .env: the fragment is recorded
// as "@plugins/", keeps its entry across a build that does not rewrite it, a
// hand-edit of it is refused without --force, and removing the plugin prunes
// the entry.
func TestPluginDirOnlyInProjectEnv(t *testing.T) {
	for _, tc := range shellPluginDirs {
		t.Run(tc.name, func(t *testing.T) {
			f := loadFixture(t, "dev-plugin")
			env := readFile(t, filepath.Join(f.project, ".env"))
			writeFile(t, filepath.Join(f.project, ".env"), env+"NSELF_PLUGIN_DIR="+f.plugins+"\n", 0o600)
			frag := filepath.Join(f.plugins, "nself-alpha", "docker-compose.plugin.yml")
			raw := readFile(t, frag)
			key := PluginsPrefix + "nself-alpha/docker-compose.plugin.yml"
			// A write-mode build loads the project env into the process and
			// keeps it, so the shell state is put back before every step.
			apply := func(opt ApplyOptions) error {
				t.Helper()
				tc.shell(t)
				defer tc.shell(t)
				_, err := Apply(context.Background(), Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("fixture-seed")}, opt)
				return err
			}
			recorded := func() bool {
				t.Helper()
				s, err := LoadGeneratedState(f.project)
				if err != nil || s == nil {
					t.Fatalf("record: %v", err)
				}
				_, ok := s.Files[key]
				return ok
			}

			if err := apply(ApplyOptions{Yes: true}); err != nil {
				t.Fatal(err)
			}
			if !recorded() {
				t.Fatalf("the rewritten fragment must be recorded as %s", key)
			}
			normalized := readFile(t, frag)
			if normalized == raw {
				t.Fatal("fixture precondition: the first build must rewrite the fragment")
			}

			// A build that does not rewrite the fragment keeps its entry.
			if err := apply(ApplyOptions{Yes: true}); err != nil {
				t.Fatal(err)
			}
			if !recorded() || readFile(t, frag) != normalized {
				t.Fatal("an installed plugin's fragment must keep its record entry")
			}

			// A hand-edit the build would overwrite is refused without --force.
			writeFile(t, frag, raw, 0o644)
			compattest.Set(t, true)
			wantE403(t, apply(ApplyOptions{Yes: true}))
			if readFile(t, frag) != raw {
				t.Fatal("a refused apply overwrote the hand-edited fragment")
			}

			// A removed plugin leaves the record.
			if err := os.RemoveAll(filepath.Join(f.plugins, "nself-alpha")); err != nil {
				t.Fatal(err)
			}
			if err := apply(ApplyOptions{Yes: true}); err != nil {
				t.Fatal(err)
			}
			if recorded() {
				t.Fatalf("a removed plugin's %s must leave the record", key)
			}
		})
	}
}
