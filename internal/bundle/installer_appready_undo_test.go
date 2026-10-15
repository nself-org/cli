package bundle

// installer_appready_undo_test.go — D-0760: a failed or refused reconcile after a
// bundle install undoes the members that install added and their ledger records.

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle/appready"
	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/config"
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
	pluginDir := isolate(t)
	setupOfflineRegistry(t, mockTaskRegistry())
	var out bytes.Buffer
	opts := freeOpts(t, pluginDir, &out, func(name string) error {
		writeMember(t, pluginDir, name, 1, nil)
		return nil
	})
	opts.Hooks = refusedHooks(t)

	res, err := Install(context.Background(), "task", opts)
	if err == nil || !errors.Is(err, errRefused) || !strings.Contains(err.Error(), "reconcile failed after install") {
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

// TestBundleInstallUndoFailureIsReported: when a member cannot be undone the
// reconcile error is still returned, joined with the undo failure, and the
// record stays so ledger and disk agree.
func TestBundleInstallUndoFailureIsReported(t *testing.T) {
	pluginDir := isolate(t)
	setupOfflineRegistry(t, mockNotificationsOnly())
	var out bytes.Buffer
	opts := freeOpts(t, pluginDir, &out, func(name string) error {
		writeMember(t, pluginDir, name, 1, nil)
		return nil
	})
	opts.Hooks = refusedHooks(t)
	opts.undoer = func(context.Context, *config.Config, string, string) error { return errors.New("disk is read-only") }

	_, err := Install(context.Background(), "task", opts)
	if !errors.Is(err, errRefused) || !strings.Contains(err.Error(), "disk is read-only") {
		t.Fatalf("err = %v, want the reconcile error joined with the undo failure", err)
	}
	l, _, _ := ledger.NewStore(pluginDir).Load()
	if _, ok := l.Plugins["notifications"]; !ok {
		t.Error("notifications left the ledger although it is still on disk")
	}
}
