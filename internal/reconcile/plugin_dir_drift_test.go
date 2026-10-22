package reconcile

import (
	nbuild "github.com/nself-org/cli/internal/build"
	"os"
	"path/filepath"
	"testing"
)

func TestHandEditedUsesProjectPluginCascade(t *testing.T) {
	for _, layer := range []string{".env", ".env.local"} {
		t.Run(layer, func(t *testing.T) {
			project, plugins := t.TempDir(), t.TempDir()
			t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
			if err := os.WriteFile(filepath.Join(project, layer), []byte("NSELF_PLUGIN_DIR="+plugins+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(plugins, "fragment.yml"), []byte("changed"), 0600); err != nil {
				t.Fatal(err)
			}
			state := &GeneratedState{Files: map[string]string{PluginsPrefix + "fragment.yml": "old"}}
			if !state.HandEditedFn(project)(PluginsPrefix + "fragment.yml") {
				t.Fatal("missed project plugin drift")
			}
		})
	}
}

func TestPlannedRecordRekeysAbsolutePluginFragments(t *testing.T) {
	for _, planned := range []bool{false, true} {
		plugins := t.TempDir()
		absolute := filepath.Join(plugins, "p", "docker-compose.plugin.yml")
		old := AbsPrefix + filepath.ToSlash(absolute)[1:]
		state := &GeneratedState{Files: map[string]string{old: "old", AbsPrefix + "unrelated/file": "other"}}
		pb := &nbuild.PlannedBuild{PluginDir: plugins, Files: map[string]nbuild.PlannedFile{}}
		if planned {
			pb.Files[absolute] = nbuild.PlannedFile{}
		}
		next, _, err := PlannedRecord(state, pb)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := next.Files[old]; ok {
			t.Error("stale absolute plugin key remains")
		}
		if _, ok := next.Files[AbsPrefix+"unrelated/file"]; !ok {
			t.Error("unrelated absolute key lost")
		}
		if _, ok := state.Files[old]; !ok {
			t.Error("original state mutated")
		}
	}
}
