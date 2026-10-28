package reconcile

// Purpose: tests for the legacy plugin-key prune of the generated-state record
// (debts D-0557, D-0607, D-0318).
// Inputs: synthetic records and the dev-plugin fixture with the plugin
// directory set only in the project's .env (D-0607).
// Outputs: pass/fail on which "@plugins/" keys survive PlannedRecord and on
// where the build records and writes a plugin's compose fragment.
// Constraints: temp dirs only; no network.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	nbuild "github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compat/compattest"
)

// TestPruneLegacyPluginKeys: every "@plugins/" key the build is not writing
// leaves the record; project keys and planned keys stay.
func TestPruneLegacyPluginKeys(t *testing.T) {
	s := &GeneratedState{Files: map[string]string{
		"docker-compose.yml":                                "p",
		".nself/plugins/live/docker-compose.plugin.yml":     "o",
		PluginsPrefix + "live/docker-compose.plugin.yml":    "l",
		PluginsPrefix + "gone/extra.yml":                    "g2",
		PluginsPrefix + "planned/docker-compose.plugin.yml": "w",
		PluginsPrefix + "flat.yml":                          "f",
	}}
	s.pruneLegacyPluginKeys(map[string]bool{PluginsPrefix + "planned/docker-compose.plugin.yml": true}, "")

	for _, k := range []string{"docker-compose.yml", ".nself/plugins/live/docker-compose.plugin.yml", PluginsPrefix + "planned/docker-compose.plugin.yml"} {
		if _, ok := s.Files[k]; !ok {
			t.Errorf("%s must stay in the record", k)
		}
	}
	for _, k := range []string{PluginsPrefix + "live/docker-compose.plugin.yml", PluginsPrefix + "gone/extra.yml", PluginsPrefix + "flat.yml"} {
		if _, ok := s.Files[k]; ok {
			t.Errorf("%s is a legacy key and must leave the record", k)
		}
	}
}

// TestPlannedRecordDropsLegacyPluginKeys: through PlannedRecord a legacy
// fragment key leaves the record whether or not the plugin is installed, and
// the project-local overlay key stays. Folding twice gives the same bytes, so
// a plan after the apply does not list the record again.
func TestPlannedRecordDropsLegacyPluginKeys(t *testing.T) {
	isolateEnv(t)
	plugins := t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", plugins)
	if err := os.MkdirAll(filepath.Join(plugins, "kept"), 0o755); err != nil {
		t.Fatal(err)
	}
	legacyKept := PluginsPrefix + "kept/docker-compose.plugin.yml"
	legacyGone := PluginsPrefix + "removed/docker-compose.plugin.yml"
	overlayKey := ".nself/plugins/kept/docker-compose.plugin.yml"
	cur := &GeneratedState{Files: map[string]string{legacyKept: "k", legacyGone: "r", overlayKey: "o", "docker-compose.yml": "c"}}
	pb := &nbuild.PlannedBuild{Files: map[string]nbuild.PlannedFile{}, PluginDir: plugins}

	next, body, err := PlannedRecord(cur, pb)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{legacyKept, legacyGone} {
		if _, ok := next.Files[k]; ok {
			t.Fatalf("legacy key %s must leave the record: %v", k, next.Files)
		}
	}
	for _, k := range []string{overlayKey, "docker-compose.yml"} {
		if _, ok := next.Files[k]; !ok {
			t.Fatalf("%s must stay in the record: %v", k, next.Files)
		}
	}
	if _, ok := cur.Files[legacyGone]; !ok {
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

// TestPluginDirOnlyInProjectEnv (D-0607, D-0318) runs the real plan and apply
// with NSELF_PLUGIN_DIR set only in the project's .env: the normalized
// fragment is written and recorded as a project file (.nself/plugins/), the
// shared fragment stays byte-identical, and a hand-edit of the overlay is
// refused without --force.
func TestPluginDirOnlyInProjectEnv(t *testing.T) {
	for _, tc := range shellPluginDirs {
		t.Run(tc.name, func(t *testing.T) {
			f := loadFixture(t, "dev-plugin")
			env := readFile(t, filepath.Join(f.project, ".env"))
			writeFile(t, filepath.Join(f.project, ".env"), env+"NSELF_PLUGIN_DIR="+f.plugins+"\n", 0o600)
			frag := filepath.Join(f.plugins, "nself-alpha", "docker-compose.plugin.yml")
			raw := readFile(t, frag)
			key := ".nself/plugins/nself-alpha/docker-compose.plugin.yml"
			overlay := filepath.Join(f.project, filepath.FromSlash(key))
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
				t.Fatalf("the normalized fragment must be recorded as %s", key)
			}
			normalized := readFile(t, overlay)
			if normalized == raw {
				t.Fatal("fixture precondition: the first build must normalize the fragment")
			}
			if readFile(t, frag) != raw {
				t.Fatal("the build rewrote the shared plugin fragment")
			}

			// A second build keeps the overlay and still leaves the shared file alone.
			if err := apply(ApplyOptions{Yes: true}); err != nil {
				t.Fatal(err)
			}
			if !recorded() || readFile(t, overlay) != normalized || readFile(t, frag) != raw {
				t.Fatal("a rebuild must keep the overlay and leave the shared fragment alone")
			}

			// A hand-edit the build would overwrite is refused without --force.
			edited := normalized + "# hand edit\n"
			writeFile(t, overlay, edited, 0o644)
			compattest.Set(t, true)
			wantE403(t, apply(ApplyOptions{Yes: true}))
			if got := readFile(t, overlay); got != edited || strings.Contains(readFile(t, frag), "hand edit") {
				t.Fatal("a refused apply overwrote the hand-edited overlay")
			}
		})
	}
}

// TestPlannedRecordDropsLegacyAbsPluginKeys (D-0644): a plugin fragment that
// an old record named "@abs/<path>" (the shell's plugin directory differed from
// the build's) is under the build's plugin directory, so it leaves the record
// like its "@plugins/" twin. An "@abs/" key outside the plugin directory stays.
func TestPlannedRecordDropsLegacyAbsPluginKeys(t *testing.T) {
	isolateEnv(t)
	plugins := t.TempDir()
	abs := func(p string) string { return AbsPrefix + strings.TrimPrefix(filepath.ToSlash(p), "/") }
	legacy := abs(filepath.Join(plugins, "alpha", "docker-compose.plugin.yml"))
	outside := abs(filepath.Join(t.TempDir(), "elsewhere.conf"))
	cur := &GeneratedState{Files: map[string]string{legacy: "old", outside: "keep", "docker-compose.yml": "c"}}
	pb := &nbuild.PlannedBuild{Files: map[string]nbuild.PlannedFile{}, PluginDir: plugins}

	next, body, err := PlannedRecord(cur, pb)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := next.Files[legacy]; ok {
		t.Fatalf("legacy @abs/ plugin key must leave the record: %v", next.Files)
	}
	for _, k := range []string{outside, "docker-compose.yml"} {
		if _, ok := next.Files[k]; !ok {
			t.Fatalf("%s must stay in the record: %v", k, next.Files)
		}
	}
	if _, ok := cur.Files[legacy]; !ok {
		t.Fatal("PlannedRecord must not mutate the current record")
	}
	if _, again, err := PlannedRecord(next, pb); err != nil || string(again) != string(body) {
		t.Fatalf("folding twice must be idempotent: %v", err)
	}
}
