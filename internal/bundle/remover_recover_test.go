package bundle

// remover_recover_test.go — D-0368: a remove intent whose bundle the ledger no
// longer records is resumed (members on disk) or cleared (stale), never E134
// for work this command started itself.

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
)

var recoverStart = time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)

// writeIntent records a family intent with the given remaining members.
func writeIntent(t *testing.T, dir string, remaining ...string) {
	t.Helper()
	err := newLedgerStore(dir).Locked(func(tx *ledger.Tx, _ ledger.Ledger) error {
		return tx.SetRemoveIntent("family", ledger.RemoveIntent{StartedAt: recoverStart, Remaining: remaining})
	})
	if err != nil {
		t.Fatal(err)
	}
}

// ledgerWithout writes a ledger that has no family bundle and no family records.
func ledgerWithout(t *testing.T, dir string) {
	t.Helper()
	err := newLedgerStore(dir).Update(func(l *ledger.Ledger) error {
		l.Bundles, l.Plugins = map[string]ledger.BundleRecord{}, map[string]ledger.PluginRecord{}
		l.Plugins["unrelated"] = ledger.PluginRecord{InstalledBy: []string{}, Explicit: true, Tier: ledger.TierFree, Version: "1.0.0"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecoverLostBundleResumesRemainingMembers(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		useOverlapBundles(t)
		dir := t.TempDir()
		installPlugins(t, dir, "d1", "d2") // d3 was removed before the interruption
		ledgerWithout(t, dir)
		writeIntent(t, dir, "d1", "d2")
		var out bytes.Buffer
		rec := &recorder{}
		res, err := Remove(context.Background(), "family", baseOpts(dir, &out, rec))
		if err != nil {
			t.Fatalf("remove: %v\n%s", err, out.String())
		}
		if !sameStrings(res.Removed, []string{"d2", "d1"}) {
			t.Errorf("removed %v, want [d2 d1]", res.Removed)
		}
		if !strings.Contains(out.String(), "resuming an interrupted remove of bundle family (started 2026-10-10T08:00:00Z)") {
			t.Errorf("no resume line:\n%s", out.String())
		}
		l := loadLedger(t, dir)
		if _, ok := l.Bundles["family"]; ok || len(l.Plugins) != 1 {
			t.Errorf("ledger = %+v, want only the unrelated record", l)
		}
		if intentFileExists(dir) {
			t.Error("intent file survived")
		}
	})
}

// A re-bootstrap marks leftover plugins explicit. The restore must not touch
// that flag, so they are kept and nothing is deleted on a guess.
func TestRecoverKeepsExplicitFlags(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		useOverlapBundles(t)
		dir := t.TempDir()
		installPlugins(t, dir, "d1", "d2")
		ledgerWithout(t, dir)
		err := newLedgerStore(dir).Update(func(l *ledger.Ledger) error {
			for _, n := range []string{"d1", "d2"} {
				l.Plugins[n] = ledger.PluginRecord{InstalledBy: []string{}, Explicit: true, Tier: ledger.TierFree, Version: "1.0.0"}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		writeIntent(t, dir, "d1", "d2")
		var out bytes.Buffer
		rec := &recorder{}
		res, err := Remove(context.Background(), "family", baseOpts(dir, &out, rec))
		if err != nil || len(rec.calls) != 0 || len(res.Kept) != 2 {
			t.Fatalf("err=%v calls=%v kept=%+v; want two explicit members kept\n%s", err, rec.names(), res.Kept, out.String())
		}
		l := loadLedger(t, dir)
		if !l.Plugins["d1"].Explicit || !l.Plugins["d2"].Explicit {
			t.Error("explicit flags were changed")
		}
		if _, ok := l.Bundles["family"]; ok || intentFileExists(dir) {
			t.Error("bundle or intent left behind")
		}
	})
}

func TestRecoverStaleIntentIsCleared(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		useOverlapBundles(t)
		dir := t.TempDir()
		installPlugins(t, dir, "unrelated")
		ledgerWithout(t, dir)
		writeIntent(t, dir, "d1", "d2") // neither is on disk
		var out bytes.Buffer
		rec := &recorder{}
		_, err := Remove(context.Background(), "family", baseOpts(dir, &out, rec))
		if intentFileExists(dir) {
			t.Error("the stale intent was not cleared")
		}
		if len(rec.calls) != 0 {
			t.Errorf("removed %v", rec.names())
		}
		if compat.V15() {
			if d := errs.Describe(err); d == nil || d.Code != "E134" {
				t.Errorf("v1.5: err = %v, want the normal E134 path", err)
			}
		} else if err != nil || !strings.Contains(out.String(), "not recorded") {
			t.Errorf("v1.4: err=%v out=%s, want the no-op warning", err, out.String())
		}
	})
}

func TestRecoverBadIntentRefusesBeforeRemoving(t *testing.T) {
	dir := familyDir(t)
	p := newLedgerStore(dir).IntentPath()
	// The ledger directory exists once a store has been used.
	if err := newLedgerStore(dir).Update(func(*ledger.Ledger) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`{"_generated":"hand","schema_version":1,"bundles":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	rec := &recorder{}
	_, err := Remove(context.Background(), "family", baseOpts(dir, &out, rec))
	if err == nil || !strings.Contains(err.Error(), p) {
		t.Fatalf("err = %v, want it to name %s", err, p)
	}
	if len(rec.calls) != 0 {
		t.Errorf("removed %v with a damaged intent file", rec.names())
	}
}
