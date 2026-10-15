package bundle

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle/ledger"
)

func TestRemoveRefusesUnreadableOutsideManifest(t *testing.T) {
	dir := chatDir(t)
	if err := os.WriteFile(filepath.Join(dir, "outsider", "plugin.json"), []byte(`{broken`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, rec, err := removeChat(t, dir, nil)
	if err == nil || !strings.Contains(err.Error(), "outsider") {
		t.Fatalf("remove error = %v; want unreadable outsider", err)
	}
	if len(rec.calls) != 0 || !isAlreadyInstalled(dir, "px") || !isAlreadyInstalled(dir, "py") {
		t.Fatalf("dependency scan failure removed members: %v", rec.names())
	}
}

func TestRemoveKeepsDependencyDespiteManifestNameCollision(t *testing.T) {
	dir := chatDir(t)
	setDeps(t, dir, "outsider", "px")
	path := filepath.Join(dir, "outsider", "plugin.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Keep the schema consistent with the renamed manifest so it stays valid.
	if err := os.WriteFile(path, bytes.ReplaceAll(raw, []byte("outsider"), []byte("py")), 0o644); err != nil {
		t.Fatal(err)
	}
	res, out, rec, err := removeChat(t, dir, nil)
	if err != nil {
		t.Fatalf("remove: %v\n%s", err, out)
	}
	if !sameStrings(res.Removed, []string{"py"}) || !sameStrings(rec.names(), []string{"py"}) || !isAlreadyInstalled(dir, "px") {
		t.Fatalf("removed %v; want py only, keeping outsider's px dependency", rec.names())
	}
}

func TestInstallClearsStaleRemoveIntent(t *testing.T) {
	pluginDir := isolate(t)
	useOverlapBundles(t)
	setupOfflineRegistry(t, []byte(`{"version":"test","tier":"free","plugins":{
	"px":{"name":"px","version":"1.0.0","status":"stable","tier":"free"},
	"py":{"name":"py","version":"1.0.0","status":"stable","tier":"free"}}}`))
	installPlugins(t, pluginDir, "px", "py")
	recordBundle(t, pluginDir, "chat")
	writeIntent(t, pluginDir, "d1")
	if err := newLedgerStore(pluginDir).Locked(func(tx *ledger.Tx, _ ledger.Ledger) error {
		return tx.SetRemoveIntent("chat", ledger.RemoveIntent{StartedAt: recoverStart, Remaining: []string{"px"}})
	}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	opts := paidOpts(pluginDir, &out, func(name string) error { writeMember(t, pluginDir, name, 1, nil); return nil })
	if _, err := Install(context.Background(), "chat", opts); err != nil {
		t.Fatalf("install: %v\n%s", err, out.String())
	}
	if _, ok := intentOf(t, pluginDir, "chat"); ok {
		t.Fatal("successful install left the stale remove intent")
	}
	if _, ok := intentOf(t, pluginDir, "family"); !ok {
		t.Fatal("install cleared another bundle's remove intent")
	}
}
