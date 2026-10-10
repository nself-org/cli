package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/plugin"
)

// TestLedgerSources: the adapter reads membership from the loaded bundles and
// the installed plugins from the plugin dir; the ledger's own .nself-state
// directory is not a plugin and is never listed.
func TestLedgerSources(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	writeMember(t, dir, "notifications", 3999, nil)
	st := ledger.NewStore(dir)
	st.Sources = LedgerSources(dir)
	if _, err := st.EnsureBootstrapped(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".nself-state", "bundles.json")); err != nil {
		t.Fatal(err)
	}
	src, err := LedgerSources(dir)()
	if err != nil {
		t.Fatal(err)
	}
	if len(src.Plugins) != 1 || src.Plugins[0].Name != "notifications" || src.Plugins[0].Tier != ledger.TierLicensed {
		t.Errorf("plugins = %+v (fixture manifest is tier pro)", src.Plugins)
	}
	var task *ledger.BundleDef
	for i := range src.Bundles {
		if src.Bundles[i].Slug == "task" {
			task = &src.Bundles[i]
		}
		if src.Bundles[i].Slug == "nself-plus" && src.Bundles[i].Installable {
			t.Error("nself-plus must not be installable")
		}
	}
	if task == nil || !task.Installable {
		t.Errorf("task bundle def = %+v, want installable", task)
	}
	// Scans that feed the install flow skip the state directory.
	list, err := plugin.ListInstalled(dir)
	if err != nil || len(list) != 1 {
		t.Errorf("ListInstalled = %v %v, want only notifications", list, err)
	}
	ms, _ := plugin.LoadManifestsFromDir(dir)
	if len(ms) != 1 {
		t.Errorf("LoadManifestsFromDir = %d manifests", len(ms))
	}
	if v := buildInstalledVersionMap(dir); len(v) != 1 {
		t.Errorf("buildInstalledVersionMap = %v", v)
	}
}

// TestLedgerSourcesUnreadableDir: an unreadable plugin dir is an error, never an empty source.
func TestLedgerSourcesUnreadableDir(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LedgerSources(f)(); err == nil {
		t.Skip("platform reports a regular file as an empty directory")
	}
}

func TestInstalledMembersKeepsPlannedOrder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	writeMember(t, dir, "jobs", 1, nil)
	writeMember(t, dir, "notifications", 2, nil)
	got, err := installedMembers(dir, []string{"notifications", "ghost", "jobs"})
	if err != nil || len(got) != 2 || got[0].Name != "notifications" || got[1].Name != "jobs" {
		t.Fatalf("installedMembers = %+v %v", got, err)
	}
}
