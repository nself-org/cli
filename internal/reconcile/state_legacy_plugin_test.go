package reconcile

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	nbuild "github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestLegacyPluginRecordMigration(t *testing.T) {
	plugins := t.TempDir()
	live := filepath.Join(plugins, "installed", "fragment.yml")
	writeFile(t, live, "current", 0o644)
	legacy := AbsPrefix + strings.TrimPrefix(filepath.ToSlash(live), "/")
	missing := AbsPrefix + strings.TrimPrefix(filepath.ToSlash(filepath.Join(plugins, "installed", "gone.yml")), "/")
	outside := AbsPrefix + strings.TrimPrefix(filepath.ToSlash(plugins+"-other/fragment.yml"), "/")
	cur := &GeneratedState{Files: map[string]string{legacy: "old", missing: "gone", outside: "unrelated"}}
	pb := &nbuild.PlannedBuild{PluginDir: plugins}
	next, body, err := PlannedRecord(cur, pb)
	if err != nil {
		t.Fatal(err)
	}
	if next.Files[PluginsPrefix+"installed/fragment.yml"] != "old" || len(next.Files) != 2 || next.Files[outside] != "unrelated" {
		t.Fatalf("legacy plugin keys were not migrated/pruned: %v", next.Files)
	}
	if cur.Files[legacy] != "old" || len(cur.Files) != 3 {
		t.Fatal("migration changed the loaded record")
	}
	_, again, err := PlannedRecord(next, pb)
	if err != nil || string(body) != string(again) {
		t.Fatalf("migration is not idempotent: %v", err)
	}
	cur.Files[PluginsPrefix+"installed/fragment.yml"] = "canonical"
	next, _, err = PlannedRecord(cur, pb)
	if err != nil || next.Files[PluginsPrefix+"installed/fragment.yml"] != "canonical" {
		t.Fatalf("legacy key replaced canonical hash: %v", err)
	}
}

func TestLegacyPluginHandEditRefusedBeforeMigration(t *testing.T) {
	f := loadFixture(t, "dev-plugin")
	env := readFile(t, filepath.Join(f.project, ".env"))
	writeFile(t, filepath.Join(f.project, ".env"), env+"NSELF_PLUGIN_DIR="+f.plugins+"\n", 0o600)
	frag := filepath.Join(f.plugins, "nself-alpha", "docker-compose.plugin.yml")
	raw := readFile(t, frag)
	req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("fixture-seed")}
	if _, err := Apply(context.Background(), req, ApplyOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	state, err := LoadGeneratedState(f.project)
	if err != nil {
		t.Fatal(err)
	}
	key := PluginsPrefix + "nself-alpha/docker-compose.plugin.yml"
	legacy := AbsPrefix + strings.TrimPrefix(filepath.ToSlash(frag), "/")
	state.Files[legacy] = state.Files[key]
	delete(state.Files, key)
	if err := state.Save(f.project); err != nil {
		t.Fatal(err)
	}
	writeFile(t, frag, raw, 0o644)
	t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
	compattest.Set(t, true)
	_, err = Apply(context.Background(), req, ApplyOptions{Yes: true})
	wantE403(t, err)
	if readFile(t, frag) != raw {
		t.Fatal("upgrade overwrote a legacy-recorded hand edit")
	}
	loaded, err := LoadGeneratedState(f.project)
	if err != nil || loaded.Files[legacy] == "" {
		t.Fatalf("refusal rewrote the record: %v", err)
	}
}
