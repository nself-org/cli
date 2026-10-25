package bundle

// installer_intent_test.go — D-0662: installing a bundle supersedes an old
// remove intent of that bundle, so a later remove does not print "resuming".

import (
	"bytes"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/bundle/ledger"
)

func TestBundleInstallClearsStaleRemoveIntent(t *testing.T) {
	pluginDir := isolate(t)
	setupOfflineRegistry(t, mockNotificationsOnly())
	store := newLedgerStore(pluginDir)
	for _, slug := range []string{"task", "other"} {
		err := store.Locked(func(tx *ledger.Tx, _ ledger.Ledger) error {
			return tx.SetRemoveIntent(slug, ledger.RemoveIntent{StartedAt: time.Now(), Remaining: []string{"notifications"}})
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	opts := freeOpts(t, pluginDir, &out, func(name string) error {
		writeMember(t, pluginDir, name, 1, nil)
		return nil
	})
	if _, err := Install(opCtx(t), "task", opts); err != nil {
		t.Fatalf("install: %v\n%s", err, out.String())
	}
	if _, ok := intentOf(t, pluginDir, "task"); ok {
		t.Error("the install left task's stale remove intent")
	}
	if _, ok := intentOf(t, pluginDir, "other"); !ok {
		t.Error("the install cleared another bundle's remove intent")
	}
}
