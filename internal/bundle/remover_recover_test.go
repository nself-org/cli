package bundle

// remover_recover_test.go — D-0368: an intent whose bundle the ledger does not
// record is only cleared; the normal path follows (E134 on v1.5, a no-op
// warning on v1.4) and nothing is removed or rebuilt from the intent.

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

// expectUnrecorded asserts the normal unrecorded-bundle outcome of a Remove.
func expectUnrecorded(t *testing.T, err error, out string) {
	t.Helper()
	if compat.V15() {
		if d := errs.Describe(err); d == nil || d.Code != "E134" {
			t.Errorf("v1.5: err = %v, want E134", err)
		}
	} else if err != nil || !strings.Contains(out, "not recorded") {
		t.Errorf("v1.4: err=%v out=%s, want the no-op warning", err, out)
	}
}

func TestRecoverLostBundleOnlyClearsTheIntent(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		useOverlapBundles(t)
		dir := t.TempDir()
		installPlugins(t, dir, "d1", "d2") // d3 was removed before the interruption
		ledgerWithout(t, dir)
		writeIntent(t, dir, "d1", "d2")
		var out bytes.Buffer
		rec := &recorder{}
		_, err := Remove(context.Background(), "family", baseOpts(dir, &out, rec))
		expectUnrecorded(t, err, out.String())
		if len(rec.calls) != 0 || !isAlreadyInstalled(dir, "d1") || !isAlreadyInstalled(dir, "d2") {
			t.Errorf("removed %v from an unrecorded bundle", rec.names())
		}
		if strings.Contains(out.String(), "resuming") {
			t.Errorf("resume line for an unrecorded bundle:\n%s", out.String())
		}
		if intentFileExists(dir) {
			t.Error("the stale intent was not cleared")
		}
		if _, ok := loadLedger(t, dir).Bundles["family"]; ok {
			t.Error("the bundle was rebuilt from the intent")
		}
	})
}

// The ledger file is deleted after a partial remove: the re-bootstrap marks the
// leftovers explicit, so the rerun keeps them and removes nothing.
func TestRecoverDeletedLedgerKeepsLeftovers(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		dir := familyDir(t)
		crashBetweenMembers(t, dir, false)
		if err := os.Remove(newLedgerStore(dir).Path()); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		rec := &recorder{}
		_, err := Remove(context.Background(), "family", baseOpts(dir, &out, rec))
		expectUnrecorded(t, err, out.String())
		if len(rec.calls) != 0 || !isAlreadyInstalled(dir, "d1") || !isAlreadyInstalled(dir, "d2") {
			t.Errorf("removed %v after the ledger was deleted", rec.names())
		}
		l := loadLedger(t, dir)
		if !l.Plugins["d1"].Explicit || !l.Plugins["d2"].Explicit {
			t.Errorf("leftovers are not explicit: %+v", l.Plugins)
		}
		if intentFileExists(dir) {
			t.Error("the stale intent survived")
		}
	})
}

// A dry run and a real run agree on an intent that names an unrecorded bundle;
// only the real run clears the intent.
func TestRecoverDryRunAndRealRunAgree(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		useOverlapBundles(t)
		dir := t.TempDir()
		installPlugins(t, dir, "d1", "d2")
		ledgerWithout(t, dir)
		writeIntent(t, dir, "d1", "d2")
		var dry, real bytes.Buffer
		opts := baseOpts(dir, &dry, &recorder{})
		opts.DryRun = true
		_, dryErr := Remove(context.Background(), "family", opts)
		expectUnrecorded(t, dryErr, dry.String())
		if !intentFileExists(dir) {
			t.Fatal("a dry run cleared the intent")
		}
		_, realErr := Remove(context.Background(), "family", baseOpts(dir, &real, &recorder{}))
		expectUnrecorded(t, realErr, real.String())
		if (dryErr == nil) != (realErr == nil) {
			t.Errorf("dry run err=%v, real run err=%v", dryErr, realErr)
		}
		if intentFileExists(dir) {
			t.Error("the real run left the stale intent")
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
		expectUnrecorded(t, err, out.String())
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
