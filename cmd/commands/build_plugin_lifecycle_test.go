package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// lifecycleFixture builds a plugin dir with plugin "foo" on disk and, when
// referenced is true, a ledger whose recorded bundle "task" references it. It
// swaps the remove seam for a fake that deletes the dir and counts calls.
func lifecycleFixture(t *testing.T, referenced bool) (dir string, calls *int) {
	t.Helper()
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)
	dir = t.TempDir()
	stageWritePlugin(t, dir, "foo")
	store := bundle.LedgerStore(dir)
	if err := store.Update(func(l *ledger.Ledger) error {
		if referenced {
			l.AddBundle("task", []ledger.InstalledPlugin{{Name: "foo", Version: "1.0.0", Tier: ledger.TierFree}}, time.Now())
		} else {
			l.AddBundle("task", []ledger.InstalledPlugin{{Name: "other", Version: "1.0.0", Tier: ledger.TierFree}}, time.Now())
			l.Plugins["foo"] = ledger.PluginRecord{Version: "1.0.0", Tier: ledger.TierFree, Explicit: true}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	n := 0
	prev := lifecycleRemovePlugin
	lifecycleRemovePlugin = func(_ context.Context, _ *config.Config, name, pluginDir string, _, _ bool) error {
		n++
		return os.RemoveAll(filepath.Join(pluginDir, name))
	}
	t.Cleanup(func() { lifecycleRemovePlugin = prev })
	return dir, &n
}

// ledgerHas reports whether the ledger of dir still records the plugin.
func ledgerHas(t *testing.T, dir, name string) bool {
	t.Helper()
	l, _, err := bundle.LedgerStore(dir).Load()
	if err != nil {
		t.Fatal(err)
	}
	_, ok := l.Plugins[name]
	return ok
}

// TestLifecycleRemoveBundleGuard: v1.5 refuses a bundle member with E133 and
// changes nothing; v1.4 warns, removes and forgets the record.
func TestLifecycleRemoveBundleGuard(t *testing.T) {
	t.Run("v1.5 refuses", func(t *testing.T) {
		compattest.Set(t, true)
		dir, calls := lifecycleFixture(t, true)
		var w bytes.Buffer
		err := removeExpiredPlugin(&w, &config.Config{}, dir, "foo")
		if err == nil || errs.Describe(err).Code != "E133" {
			t.Fatalf("err = %v, want E133", err)
		}
		if *calls != 0 || !stageExists(dir, "foo") || !ledgerHas(t, dir, "foo") {
			t.Errorf("a refused removal changed state: calls=%d dir=%v record=%v", *calls, stageExists(dir, "foo"), ledgerHas(t, dir, "foo"))
		}
	})
	t.Run("v1.4 warns and removes", func(t *testing.T) {
		compattest.Set(t, false)
		dir, calls := lifecycleFixture(t, true)
		var w bytes.Buffer
		if err := removeExpiredPlugin(&w, &config.Config{}, dir, "foo"); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(w.String(), "still referenced by bundle(s) task") {
			t.Errorf("no warning, got %q", w.String())
		}
		if *calls != 1 || stageExists(dir, "foo") || ledgerHas(t, dir, "foo") {
			t.Errorf("calls=%d dir=%v record=%v", *calls, stageExists(dir, "foo"), ledgerHas(t, dir, "foo"))
		}
	})
}

// TestLifecycleRemoveUnreferenced: with no bundle reference, both modes remove
// the plugin, forget its record, and print no warning.
func TestLifecycleRemoveUnreferenced(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		dir, calls := lifecycleFixture(t, false)
		var w bytes.Buffer
		if err := removeExpiredPlugin(&w, &config.Config{}, dir, "foo"); err != nil {
			t.Fatal(err)
		}
		if w.Len() != 0 {
			t.Errorf("unexpected output %q", w.String())
		}
		if *calls != 1 || stageExists(dir, "foo") || ledgerHas(t, dir, "foo") {
			t.Errorf("calls=%d dir=%v record=%v", *calls, stageExists(dir, "foo"), ledgerHas(t, dir, "foo"))
		}
	})
}
