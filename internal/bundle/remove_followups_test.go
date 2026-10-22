package bundle

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/bundle/ledger"
)

func TestRemoveRefusesUnreadableOutsideManifest(t *testing.T) {
	for _, invalid := range []string{"{broken", ""} {
		t.Run("unreadable", func(t *testing.T) {
			dir := chatDir(t)
			if err := os.WriteFile(filepath.Join(dir, "outsider", "plugin.json"), []byte(invalid), 0600); err != nil {
				t.Fatal(err)
			}
			_, _, rec, err := removeChat(t, dir, nil)
			if err == nil || len(rec.calls) != 0 {
				t.Fatal("removal proceeded without knowing outside dependencies")
			}
		})
	}
}

func TestBundleReinstallClearsOnlyOwnRemoveIntent(t *testing.T) {
	guardOperationLockTest(t)
	dir := isolate(t)
	setupOfflineRegistry(t, mockTaskRegistry())
	var out bytes.Buffer
	opts := freeOpts(t, dir, &out, func(name string) error { writeMember(t, dir, name, 3901, nil); return nil })
	opts.Hooks = nil
	if _, err := Install(opCtx(t), "task", opts); err != nil {
		t.Fatal(err)
	}
	store := ledger.NewStore(dir)
	if err := store.Locked(func(tx *ledger.Tx, _ ledger.Ledger) error {
		for _, slug := range []string{"task", "other"} {
			if err := tx.SetRemoveIntent(slug, ledger.RemoveIntent{StartedAt: time.Now(), Remaining: []string{"jobs"}}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(opCtx(t), "task", opts); err != nil {
		t.Fatal(err)
	}
	if err := store.Locked(func(tx *ledger.Tx, _ ledger.Ledger) error {
		if _, ok, err := tx.RemoveIntent("task"); err != nil || ok {
			t.Errorf("task intent remains: %v", err)
		}
		if _, ok, err := tx.RemoveIntent("other"); err != nil || !ok {
			t.Errorf("other intent lost: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
