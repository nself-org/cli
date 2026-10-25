package bundle

// installer_appready_undo_test.go — D-0760: a failed or refused reconcile after a
// bundle install undoes the members that install added and their ledger records.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/bundle/appready"
	"github.com/nself-org/cli/internal/bundle/ledger"
)

var errRefused = errors.New("E403: the build needs --yes")

func refusedHooks(t *testing.T) *appready.Hooks {
	t.Helper()
	return &appready.Hooks{
		Reconcile: func(context.Context) error { return errRefused },
		Start:     func(context.Context) error { t.Error("start ran after a refused reconcile"); return nil },
	}
}

// TestBundleInstallRefusedReconcileUndoesMembers: every fresh member dir and the
// bundle's ledger records are gone after the reconcile is refused; the error is
// still the reconcile error.
func TestBundleInstallRefusedReconcileUndoesMembers(t *testing.T) {
	for _, code := range []string{"E403", "E480", "E481"} {
		t.Run(code, func(t *testing.T) {
			pluginDir := isolate(t)
			setupOfflineRegistry(t, mockTaskRegistry())
			var out bytes.Buffer
			opts := freeOpts(t, pluginDir, &out, func(name string) error {
				writeMember(t, pluginDir, name, 1, []string{"np_kept_data"})
				return nil
			})
			opts.Hooks = refusedHooks(t)
			cause := errors.New(code + ": reconcile refused")
			opts.Hooks.Reconcile = func(context.Context) error { return cause }

			res, err := Install(context.Background(), "task", opts)
			if err == nil || !errors.Is(err, cause) || !strings.Contains(err.Error(), "reconcile failed after install") {
				t.Fatalf("err = %v, want the reconcile error", err)
			}
			for _, name := range []string{"notifications", "jobs"} {
				if _, statErr := os.Stat(filepath.Join(pluginDir, name)); !errors.Is(statErr, os.ErrNotExist) {
					t.Errorf("member %q still on disk after a refused reconcile (stat err %v)", name, statErr)
				}
			}
			l, _, lerr := ledger.NewStore(pluginDir).Load()
			if lerr != nil || len(l.Bundles) != 0 || len(l.Plugins) != 0 {
				t.Errorf("ledger after the undo: err=%v bundles=%v plugins=%v", lerr, l.Bundles, l.Plugins)
			}
			if len(res.RolledBack) != 2 || res.Changed {
				t.Errorf("RolledBack=%v Changed=%v, want both members rolled back and no change", res.RolledBack, res.Changed)
			}
		})
	}
}

// A refused upgrade has no fresh members to remove, but must still undo newly
// recorded ownership while preserving prior bundle associations.
func TestBundleInstallRefusedUpgradeRestoresOwnership(t *testing.T) {
	for _, recorded := range []bool{false, true} {
		t.Run(fmt.Sprint(recorded), func(t *testing.T) {
			pluginDir := isolate(t)
			setupOfflineRegistry(t, mockTaskRegistry())
			// Persist an empty ledger before any member exists: otherwise the
			// first Update bootstraps from the members on disk and infers a
			// "task" record, which hides a missing ownership restore.
			store := newLedgerStore(pluginDir)
			if err := store.Update(func(*ledger.Ledger) error { return nil }); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"notifications", "jobs"} {
				writeMember(t, pluginDir, name, 1, nil)
				path := filepath.Join(pluginDir, name, "plugin.json")
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, bytes.ReplaceAll(raw, []byte("1.0.0"), []byte("0.9.0")), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.Update(func(l *ledger.Ledger) error {
				members := []ledger.InstalledPlugin{{Name: "jobs", Version: "0.9.0"}, {Name: "notifications", Version: "0.9.0"}}
				l.AddBundle("existing", members, time.Now())
				if recorded {
					l.AddBundle("task", members, time.Now())
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			prior := loadLedger(t, pluginDir)
			wantBy := []string{"existing"}
			if recorded {
				wantBy = []string{"existing", "task"}
			}
			_, hasTask := prior.Bundles["task"]
			if hasTask != recorded {
				t.Fatalf("prior task bundle recorded = %v, want %v", hasTask, recorded)
			}
			for _, name := range []string{"jobs", "notifications"} {
				if got := prior.Plugins[name].InstalledBy; !reflect.DeepEqual(got, wantBy) {
					t.Fatalf("prior %s installed_by = %v, want %v", name, got, wantBy)
				}
			}
			var out bytes.Buffer
			opts := freeOpts(t, pluginDir, &out, func(name string) error { writeMember(t, pluginDir, name, 1, nil); return nil })
			opts.Hooks = refusedHooks(t)
			if _, err := Install(context.Background(), "task", opts); !errors.Is(err, errRefused) {
				t.Fatalf("install: %v", err)
			}
			after := loadLedger(t, pluginDir)
			if !reflect.DeepEqual(after.Bundles, prior.Bundles) {
				t.Errorf("bundles = %v, want %v", after.Bundles, prior.Bundles)
			}
			for name, old := range prior.Plugins {
				got := after.Plugins[name]
				if !reflect.DeepEqual(got.InstalledBy, old.InstalledBy) || got.Explicit != old.Explicit {
					t.Errorf("%s ownership = %+v, want %+v", name, got, old)
				}
				if _, err := os.Stat(filepath.Join(pluginDir, name)); err != nil {
					t.Errorf("existing member removed: %v", err)
				}
			}
		})
	}
}

// TestBundleInstallRefusedReconcileKeepsPreexistingMember: a member that was on
// disk before the install stays installed and recorded as explicit; only the
// member this install added goes.
func TestBundleInstallRefusedReconcileKeepsPreexistingMember(t *testing.T) {
	pluginDir := isolate(t)
	setupOfflineRegistry(t, mockTaskRegistry())
	writeMember(t, pluginDir, "jobs", 1, nil)
	var out bytes.Buffer
	opts := freeOpts(t, pluginDir, &out, func(name string) error {
		writeMember(t, pluginDir, name, 1, nil)
		return nil
	})
	opts.Hooks = refusedHooks(t)

	if _, err := Install(context.Background(), "task", opts); !errors.Is(err, errRefused) {
		t.Fatalf("err = %v, want the reconcile error", err)
	}
	if _, statErr := os.Stat(filepath.Join(pluginDir, "notifications")); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("fresh member notifications still on disk (stat err %v)", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(pluginDir, "jobs")); statErr != nil {
		t.Errorf("pre-existing member jobs was removed: %v", statErr)
	}
	l, _, lerr := ledger.NewStore(pluginDir).Load()
	if lerr != nil || len(l.Bundles) != 0 {
		t.Fatalf("ledger: err=%v bundles=%v, want no bundle record", lerr, l.Bundles)
	}
	if rec, ok := l.Plugins["jobs"]; !ok || !rec.Explicit || len(rec.InstalledBy) != 0 {
		t.Errorf("jobs record = %+v (present %v), want explicit with no installed_by", rec, ok)
	}
	if _, ok := l.Plugins["notifications"]; ok {
		t.Error("notifications is still in the ledger")
	}
}

// A reconcile that fails after Apply wrote the generated files (a container
// swap error) must keep the plugins and their ledger records: the committed
// configuration references them. Only a pre-commit failure is undone.
func TestBundleInstallPostCommitReconcileFailureKeepsMembers(t *testing.T) {
	pluginDir := isolate(t)
	setupOfflineRegistry(t, mockTaskRegistry())
	var out bytes.Buffer
	opts := freeOpts(t, pluginDir, &out, func(name string) error {
		writeMember(t, pluginDir, name, 1, []string{"np_kept_data"})
		return nil
	})
	cause := errors.New("orphaned containers were removed but the new services did not start")
	opts.Hooks = refusedHooks(t)
	opts.Hooks.Reconcile = func(context.Context) error { return appready.Committed(cause) }

	res, err := Install(context.Background(), "task", opts)
	if err == nil || !errors.Is(err, cause) || !strings.Contains(err.Error(), "reconcile failed after install") {
		t.Fatalf("err = %v, want the reconcile error", err)
	}
	for _, name := range []string{"notifications", "jobs"} {
		if _, statErr := os.Stat(filepath.Join(pluginDir, name)); statErr != nil {
			t.Errorf("member %q was removed after the files were committed: %v", name, statErr)
		}
	}
	l, _, lerr := ledger.NewStore(pluginDir).Load()
	if lerr != nil || len(l.Bundles) != 1 || len(l.Plugins) != 2 {
		t.Errorf("ledger after a post-commit failure: err=%v bundles=%v plugins=%v", lerr, l.Bundles, l.Plugins)
	}
	if len(res.RolledBack) != 0 || !res.Changed {
		t.Errorf("RolledBack=%v Changed=%v, want nothing rolled back and the change kept", res.RolledBack, res.Changed)
	}
}
