package bundle

// remover_intent_test.go — D-0368: the durable remove intent. A kill between
// members, a failed final ledger write and a failed member must all leave a
// ledger and an intent that a rerun of the same command finishes from.
// Fixture: family={d1,d2,d3} (testdata/overlap), removed in order d3, d2, d1,
// installed with plugin.json and NO ledger file (the bug needs the bootstrap).

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// familyDir installs d1..d3 with manifests and no ledger file.
func familyDir(t *testing.T) string {
	t.Helper()
	useOverlapBundles(t)
	dir := t.TempDir()
	installPlugins(t, dir, "d1", "d2", "d3")
	return dir
}

// crashingRemover is a recorder that removes the directory of every plugin
// except panicOn, where it panics (a killed process).
func crashingRemover(r *recorder, panicOn string) func(context.Context, *config.Config, string, string, bool, bool) error {
	return func(ctx context.Context, cfg *config.Config, name, pluginDir string, keepData, force bool) error {
		if name == panicOn {
			panic("simulated kill at " + name)
		}
		return r.fn(ctx, cfg, name, pluginDir, keepData, force)
	}
}

// removeCatching runs Remove and reports whether it panicked.
func removeCatching(opts RemoveOpts) (res *RemoveResult, err error, panicked bool) {
	defer func() {
		if recover() != nil {
			panicked = true
		}
	}()
	res, err = Remove(context.Background(), "family", opts)
	return res, err, false
}

// intentOf reads the remove intent of slug from the intent file.
func intentOf(t *testing.T, pluginDir, slug string) (ledger.RemoveIntent, bool) {
	t.Helper()
	var in ledger.RemoveIntent
	var ok bool
	err := newLedgerStore(pluginDir).Locked(func(tx *ledger.Tx, _ ledger.Ledger) error {
		var err error
		in, ok, err = tx.RemoveIntent(slug)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return in, ok
}

func intentFileExists(pluginDir string) bool {
	_, err := os.Stat(newLedgerStore(pluginDir).IntentPath())
	return err == nil
}

func baseOpts(dir string, out *bytes.Buffer, r *recorder) RemoveOpts {
	o := removeOpts(dir, out, r)
	o.KeepData = true
	return o
}

// crashBetweenMembers kills the process while removing d2 (d3 already gone).
func crashBetweenMembers(t *testing.T, dir string, noIntent bool) {
	t.Helper()
	var out bytes.Buffer
	rec := &recorder{}
	opts := baseOpts(dir, &out, rec)
	opts.remover = crashingRemover(rec, "d2")
	opts.noIntent = noIntent
	if _, _, panicked := removeCatching(opts); !panicked {
		t.Fatal("the simulated kill did not happen")
	}
	if isAlreadyInstalled(dir, "d3") || !isAlreadyInstalled(dir, "d2") || !isAlreadyInstalled(dir, "d1") {
		t.Fatalf("setup: expected only d3 gone; out:\n%s", out.String())
	}
}

func TestRemoveCrashBetweenMembersNoPriorLedger(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		dir := familyDir(t)
		crashBetweenMembers(t, dir, false)

		if ledgerBytes(t, dir) == nil {
			t.Fatal("no ledger file after the crash: the checkpoint was not written")
		}
		l := loadLedger(t, dir)
		if _, ok := l.Bundles["family"]; !ok {
			t.Fatal("the ledger does not record family after the crash")
		}
		in, ok := intentOf(t, dir, "family")
		if !ok || !sameStrings(in.Remaining, []string{"d1", "d2"}) {
			t.Fatalf("intent = %+v ok=%v, want remaining [d1 d2]", in, ok)
		}

		var out bytes.Buffer
		rec := &recorder{}
		res, err := Remove(opCtx(t), "family", baseOpts(dir, &out, rec))
		if err != nil {
			t.Fatalf("rerun: %v\n%s", err, out.String())
		}
		if !sameStrings(res.Removed, []string{"d2", "d1"}) {
			t.Errorf("rerun removed %v, want [d2 d1]", res.Removed)
		}
		if !strings.Contains(out.String(), "resuming an interrupted remove of bundle family (started ") {
			t.Errorf("no resume line:\n%s", out.String())
		}
		for _, n := range []string{"d1", "d2", "d3"} {
			if isAlreadyInstalled(dir, n) {
				t.Errorf("%s is still installed", n)
			}
		}
		l = loadLedger(t, dir)
		if _, ok := l.Bundles["family"]; ok || len(l.Plugins) != 0 {
			t.Errorf("bundle not forgotten: %+v", l)
		}
		if intentFileExists(dir) {
			t.Error("the intent file survived a full success")
		}
	})
}

// NEGATIVE CONTROL: the same scenario with the intent and the checkpoint off
// is the pre-fix behaviour. If this stops failing, the test above proves nothing.
func TestRemoveCrashBetweenMembersWithoutIntentIsTheBug(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		dir := familyDir(t)
		crashBetweenMembers(t, dir, true)
		if ledgerBytes(t, dir) != nil {
			t.Fatal("setup: the old path must leave no ledger file")
		}
		var out bytes.Buffer
		rec := &recorder{}
		opts := baseOpts(dir, &out, rec)
		opts.noIntent = true
		_, err := Remove(opCtx(t), "family", opts)
		if len(rec.calls) != 0 || !isAlreadyInstalled(dir, "d1") || !isAlreadyInstalled(dir, "d2") {
			t.Fatalf("the rerun removed %v; the bug removes nothing", rec.names())
		}
		if compat.V15() {
			if d := errs.Describe(err); d == nil || d.Code != "E134" {
				t.Errorf("v1.5: err = %v, want E134", err)
			}
		} else if err != nil || !strings.Contains(out.String(), "not recorded") {
			t.Errorf("v1.4: err=%v out=%s, want the no-op warning", err, out.String())
		}
	})
}

func TestRemoveCrashWithPriorLedger(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		dir := familyDir(t)
		recordBundle(t, dir, "family")
		crashBetweenMembers(t, dir, false)
		var out bytes.Buffer
		rec := &recorder{}
		if _, err := Remove(opCtx(t), "family", baseOpts(dir, &out, rec)); err != nil {
			t.Fatalf("rerun: %v\n%s", err, out.String())
		}
		if !sameStrings(rec.names(), []string{"d2", "d1"}) || intentFileExists(dir) {
			t.Errorf("calls=%v intent=%v", rec.names(), intentFileExists(dir))
		}
		if _, ok := loadLedger(t, dir).Bundles["family"]; ok {
			t.Error("family still recorded")
		}
	})
}

// failLedgerWrite fails the n-th write to the ledger file (1-based).
func failLedgerWrite(dir string, n int) func(string) error {
	path := newLedgerStore(dir).Path()
	seen := 0
	return func(p string) error {
		if p != path {
			return nil
		}
		if seen++; seen == n {
			return errors.New("injected ledger write failure")
		}
		return nil
	}
}

func TestRemoveFinalLedgerWriteFails(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		dir := familyDir(t)
		var out bytes.Buffer
		rec := &recorder{}
		opts := baseOpts(dir, &out, rec)
		opts.writeHook = failLedgerWrite(dir, 2) // 1 = checkpoint, 2 = final
		res, err := Remove(opCtx(t), "family", opts)
		if err == nil || !strings.Contains(err.Error(), "removed d3, d2, d1 but updating the bundle ledger") {
			t.Fatalf("err = %v, want the 'removed ... but updating the bundle ledger' wording", err)
		}
		if res == nil || len(res.Removed) != 3 {
			t.Fatalf("result = %+v", res)
		}
		if _, ok := loadLedger(t, dir).Bundles["family"]; !ok {
			t.Fatal("the on-disk ledger lost the bundle: the checkpoint did not hold")
		}
		if intentFileExists(dir) {
			t.Fatal("the intent must be cleared before the final write")
		}
		out.Reset()
		if _, err := Remove(opCtx(t), "family", baseOpts(dir, &out, &recorder{})); err != nil {
			t.Fatalf("rerun: %v\n%s", err, out.String())
		}
		l := loadLedger(t, dir)
		if _, ok := l.Bundles["family"]; ok || len(l.Plugins) != 0 || intentFileExists(dir) {
			t.Errorf("rerun left state behind: %+v intent=%v", l, intentFileExists(dir))
		}
	})
}

func TestRemoveFirstWriteFailureRemovesNothing(t *testing.T) {
	cases := []struct {
		name string
		hook func(dir string) func(string) error
	}{
		{"checkpoint", func(dir string) func(string) error { return failLedgerWrite(dir, 1) }},
		{"intent", func(dir string) func(string) error {
			ip := newLedgerStore(dir).IntentPath()
			return func(p string) error {
				if p == ip {
					return errors.New("injected intent write failure")
				}
				return nil
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			compattest.Both(t, func(t *testing.T) {
				dir := familyDir(t)
				var out bytes.Buffer
				rec := &recorder{}
				opts := baseOpts(dir, &out, rec)
				opts.writeHook = c.hook(dir)
				_, err := Remove(opCtx(t), "family", opts)
				if err == nil || !strings.Contains(err.Error(), "nothing was removed") {
					t.Fatalf("err = %v, want a nothing-was-removed error", err)
				}
				if len(rec.calls) != 0 {
					t.Errorf("removed %v despite the failed write", rec.names())
				}
				for _, n := range []string{"d1", "d2", "d3"} {
					if !isAlreadyInstalled(dir, n) {
						t.Errorf("%s was removed", n)
					}
				}
				if intentFileExists(dir) {
					t.Error("an intent file exists although its write failed")
				}
			})
		})
	}
}
