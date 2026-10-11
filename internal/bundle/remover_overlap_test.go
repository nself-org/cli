package bundle

// remover_overlap_test.go — P7-PLUG-20: refcounted bundle remove, the
// --delete-data gate, the single-plugin guard (E133), dry run and mid-failure.
// Synthetic bundles from testdata/overlap (they reuse canonical display slugs, because
// ledger bootstrap only knows those): chat={px,py}, claw={py,pz},
// task (free)={pg}, family={d1,d2,d3}. Modes are selected with compattest.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
)

func TestRemoveKeepsSharedMember(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		check := func(t *testing.T, pluginDir string) {
			t.Helper()
			var out bytes.Buffer
			rec := &recorder{}
			opts := removeOpts(pluginDir, &out, rec)
			opts.KeepData = true
			res, err := Remove(context.Background(), "chat", opts)
			if err != nil {
				t.Fatalf("remove chat: %v\n%s", err, out.String())
			}
			if !sameStrings(rec.names(), []string{"px"}) || !sameStrings(res.Removed, []string{"px"}) {
				t.Errorf("removed %v (calls %v), want only px", res.Removed, rec.names())
			}
			if len(res.Kept) != 1 || res.Kept[0].Name != "py" || !strings.Contains(res.Kept[0].Reason, "claw") {
				t.Errorf("kept = %+v, want py kept because of claw", res.Kept)
			}
			for _, n := range []string{"py", "pz"} {
				if !isAlreadyInstalled(pluginDir, n) {
					t.Errorf("%s was removed", n)
				}
			}
			if isAlreadyInstalled(pluginDir, "px") {
				t.Error("px is still installed")
			}
			l := loadLedger(t, pluginDir)
			if _, ok := l.Bundles["chat"]; ok {
				t.Error("chat is still recorded")
			}
			if _, ok := l.Plugins["px"]; ok {
				t.Error("px record is still there")
			}
			if by := l.Plugins["py"].InstalledBy; !sameStrings(by, []string{"claw"}) {
				t.Errorf("py installed_by = %v, want [claw]", by)
			}
			if by := l.Plugins["pz"].InstalledBy; !sameStrings(by, []string{"claw"}) {
				t.Errorf("pz installed_by = %v, want [claw]", by)
			}
			if err := l.Validate(); err != nil {
				t.Error(err)
			}
		}
		t.Run("one project", func(t *testing.T) {
			useOverlapBundles(t)
			dir := t.TempDir()
			installPlugins(t, dir, "px", "py", "pz")
			recordBundle(t, dir, "chat")
			recordBundle(t, dir, "claw")
			check(t, dir)
		})
		// Two projects share one plugin directory: the ledger lives with the
		// plugins, so chat recorded from project 1 and claw from project 2 are
		// both seen from either (PLUG-18 S6 / D-0284).
		t.Run("two projects share one plugin dir", func(t *testing.T) {
			useOverlapBundles(t)
			dir := t.TempDir()
			p1, p2 := t.TempDir(), t.TempDir()
			installPlugins(t, dir, "px", "py", "pz")
			t.Chdir(p1)
			recordBundle(t, dir, "chat")
			t.Chdir(p2)
			recordBundle(t, dir, "claw")
			t.Chdir(p1)
			check(t, dir)
		})
	})
}

func TestRemoveExplicitSurvives(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		useOverlapBundles(t)
		dir := t.TempDir()
		installPlugins(t, dir, "px", "py")
		if err := newLedgerStore(dir).Update(func(l *ledger.Ledger) error {
			l.MarkExplicit(ledger.InstalledPlugin{Name: "px", Version: "1.0.0", Tier: ledger.TierFree})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		recordBundle(t, dir, "chat")
		var out bytes.Buffer
		rec := &recorder{}
		opts := removeOpts(dir, &out, rec)
		opts.KeepData = true
		if _, err := Remove(context.Background(), "chat", opts); err != nil {
			t.Fatalf("remove: %v\n%s", err, out.String())
		}
		if !sameStrings(rec.names(), []string{"py"}) {
			t.Errorf("removed %v, want only py (px was installed explicitly)", rec.names())
		}
		if !isAlreadyInstalled(dir, "px") {
			t.Error("the explicit px was removed")
		}
		l := loadLedger(t, dir)
		px := l.Plugins["px"]
		if !px.Explicit || len(px.InstalledBy) != 0 {
			t.Errorf("px record = %+v, want explicit with installed_by []", px)
		}
		if _, ok := l.Bundles["chat"]; ok {
			t.Error("chat is still recorded")
		}
	})
}

func TestRemoveDeleteDataNeedsConfirm(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		type outcome struct {
			rec *recorder
			out string
			err error
			dir string
		}
		run := func(t *testing.T, bundleSlug string, mut func(*RemoveOpts)) outcome {
			t.Helper()
			useOverlapBundles(t)
			dir := t.TempDir()
			installPlugins(t, dir, "px", "py", "pg")
			recordBundle(t, dir, bundleSlug)
			before := ledgerBytes(t, dir)
			var out bytes.Buffer
			rec := &recorder{}
			opts := removeOpts(dir, &out, rec)
			mut(&opts)
			_, err := Remove(context.Background(), bundleSlug, opts)
			after := ledgerBytes(t, dir)
			if err != nil && !bytes.Equal(before, after) {
				t.Error("a refused remove changed the ledger")
			}
			return outcome{rec, out.String(), err, dir}
		}
		for name, mut := range map[string]func(*RemoveOpts){
			"no yes, not interactive": func(o *RemoveOpts) { o.DeleteData = true },
			"prompt declined":         func(o *RemoveOpts) { o.DeleteData = true; o.Confirm = func(string) bool { return false } },
		} {
			o := run(t, "chat", mut)
			rec, err, dir := o.rec, o.err, o.dir
			if err == nil || errs.ExitCodeFor(err) != 4 {
				t.Errorf("%s: err = %v (exit %d), want E403 exit 4", name, err, errs.ExitCodeFor(err))
				continue
			}
			if d := errs.Describe(err); d == nil || d.Code != "E403" {
				t.Errorf("%s: detail = %+v, want E403", name, d)
			}
			if len(rec.calls) != 0 || !isAlreadyInstalled(dir, "px") {
				t.Errorf("%s: something was removed: %v", name, rec.calls)
			}
		}
		// confirmed: --yes, or a prompt answered yes
		for name, mut := range map[string]func(*RemoveOpts){
			"yes":            func(o *RemoveOpts) { o.DeleteData = true; o.Yes = true },
			"prompt granted": func(o *RemoveOpts) { o.DeleteData = true; o.Confirm = func(string) bool { return true } },
		} {
			o := run(t, "chat", mut)
			rec := o.rec
			if o.err != nil || len(rec.calls) == 0 {
				t.Fatalf("%s: err=%v calls=%v", name, o.err, rec.calls)
			}
			for _, c := range rec.calls {
				if c.KeepData {
					t.Errorf("%s: %s removed with keepData=true, want false", name, c.Name)
				}
				if !c.Force {
					t.Errorf("%s: %s removed without force (member removal keeps the dependency bypass, R4)", name, c.Name)
				}
			}
		}
		// a dry run changes nothing and needs no confirmation
		if o := run(t, "chat", func(o *RemoveOpts) { o.DeleteData = true; o.DryRun = true }); o.err != nil || len(o.rec.calls) != 0 {
			t.Errorf("dry run with --delete-data: err=%v calls=%v", o.err, o.rec.calls)
		}

		// defaults without --delete-data
		o := run(t, "chat", func(o *RemoveOpts) {})
		rec, out, err := o.rec, o.out, o.err
		if err != nil || len(rec.calls) == 0 {
			t.Fatalf("default remove: err=%v calls=%v", err, rec.calls)
		}
		warns := strings.Count(out, "warning:")
		for _, c := range rec.calls {
			if compat.V15() {
				if !c.KeepData {
					t.Errorf("v1.5 default: %s removed with keepData=false, want data kept", c.Name)
				}
			} else if c.KeepData {
				t.Errorf("v1.4 default (paid bundle): %s removed with keepData=true, want today's data drop", c.Name)
			}
		}
		if compat.V15() && warns != 0 {
			t.Errorf("v1.5 must not warn about the default, output:\n%s", out)
		}
		if !compat.V15() && (warns != 1 || strings.Count(out, "--delete-data") != 1) {
			t.Errorf("v1.4 default: want exactly one warning naming --delete-data, output:\n%s", out)
		}
		// --keep-data (deprecated alias) keeps data in both modes, with no warning
		o = run(t, "chat", func(o *RemoveOpts) { o.KeepData = true })
		rec, out, err = o.rec, o.out, o.err
		if err != nil || len(rec.calls) == 0 || strings.Contains(out, "warning:") {
			t.Fatalf("--keep-data: err=%v calls=%v out=%s", err, rec.calls, out)
		}
		for _, c := range rec.calls {
			if !c.KeepData {
				t.Errorf("--keep-data: %s removed with keepData=false", c.Name)
			}
		}
		// the free bundle keeps data without any flag, in both modes
		o = run(t, "task", func(o *RemoveOpts) {})
		rec, out, err = o.rec, o.out, o.err
		if err != nil || !sameStrings(rec.names(), []string{"pg"}) || !rec.calls[0].KeepData || strings.Contains(out, "warning:") {
			t.Fatalf("free bundle: err=%v calls=%v out=%s", err, rec.calls, out)
		}
		// the contradictory pair is refused before anything changes
		if o := run(t, "chat", func(o *RemoveOpts) { o.DeleteData, o.KeepData, o.Yes = true, true, true }); o.err == nil {
			t.Error("--delete-data with --keep-data was accepted")
		}
	})
}

func TestRemoveGuardE133(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		useOverlapBundles(t)
		dir := t.TempDir()
		installPlugins(t, dir, "py", "pz", "other")
		recordBundle(t, dir, "claw") // claw={py,pz}; py is referenced by claw
		var w bytes.Buffer

		err := GuardPluginRemove(&w, dir, "py", false)
		if compat.V15() {
			d := errs.Describe(err)
			if err == nil || d == nil || d.Code != "E133" || errs.ExitCodeFor(err) != 1 || !strings.Contains(err.Error(), "claw") {
				t.Fatalf("v1.5: err = %v (%+v), want E133 exit 1 naming claw", err, d)
			}
		} else if err != nil || !strings.Contains(w.String(), "claw") {
			t.Fatalf("v1.4: err=%v warning=%q, want nil plus a warning naming claw", err, w.String())
		}

		w.Reset()
		if err := GuardPluginRemove(&w, dir, "py", true); err != nil || !strings.Contains(w.String(), "claw") {
			t.Fatalf("--force: err=%v warning=%q, want nil plus a warning", err, w.String())
		}
		if err := ForgetPlugin(dir, "py"); err != nil {
			t.Fatal(err)
		}
		l := loadLedger(t, dir)
		if _, ok := l.Plugins["py"]; ok {
			t.Error("py record survived ForgetPlugin")
		}
		if _, ok := l.Plugins["pz"]; !ok {
			t.Error("ForgetPlugin dropped pz as well")
		}

		// an unreferenced plugin passes silently in both modes
		w.Reset()
		if err := GuardPluginRemove(&w, dir, "other", false); err != nil || w.Len() != 0 {
			t.Errorf("unreferenced plugin: err=%v out=%q", err, w.String())
		}
		// ForgetPlugin never creates a ledger
		empty := t.TempDir()
		if err := ForgetPlugin(empty, "py"); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(newLedgerStore(empty).Path()); !os.IsNotExist(err) {
			t.Errorf("ForgetPlugin created a ledger: %v", err)
		}
	})
}

func TestRemoveDryRunAndMidFailure(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		useOverlapBundles(t)
		ctx := context.Background()

		t.Run("dry run", func(t *testing.T) {
			dir := t.TempDir()
			installPlugins(t, dir, "d1", "d2", "d3")
			recordBundle(t, dir, "family")
			before := ledgerBytes(t, dir)
			var out bytes.Buffer
			rec := &recorder{}
			opts := removeOpts(dir, &out, rec)
			opts.DryRun, opts.KeepData = true, true
			res, err := Remove(ctx, "family", opts)
			if err != nil || len(rec.calls) != 0 || len(res.Planned) != 3 {
				t.Fatalf("dry run: err=%v calls=%v planned=%v", err, rec.calls, res.Planned)
			}
			if !bytes.Equal(before, ledgerBytes(t, dir)) {
				t.Error("a dry run changed the ledger")
			}
			for _, want := range []string{"Remove plan (dry-run)", "d3", "(dry-run) no changes made."} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output lacks %q:\n%s", want, out.String())
				}
			}
		})

		t.Run("dry run creates no ledger", func(t *testing.T) {
			dir := t.TempDir()
			installPlugins(t, dir, "d1", "d2", "d3")
			var out bytes.Buffer
			rec := &recorder{}
			opts := removeOpts(dir, &out, rec)
			opts.DryRun, opts.KeepData = true, true
			if _, err := Remove(ctx, "family", opts); err != nil || len(rec.calls) != 0 {
				t.Fatalf("err=%v calls=%v\n%s", err, rec.calls, out.String())
			}
			if _, err := os.Stat(newLedgerStore(dir).Path()); !os.IsNotExist(err) {
				t.Errorf("a dry run created the ledger: %v", err)
			}
		})

		t.Run("failure on the second plugin", func(t *testing.T) {
			dir := t.TempDir()
			installPlugins(t, dir, "d1", "d2", "d3")
			recordBundle(t, dir, "family")
			var out bytes.Buffer
			rec := &recorder{failOn: "d2"} // planned order is d3, d2, d1
			opts := removeOpts(dir, &out, rec)
			opts.KeepData = true
			res, err := Remove(ctx, "family", opts)
			if err == nil || !strings.Contains(err.Error(), "d2") {
				t.Fatalf("err = %v, want a failure naming d2", err)
			}
			if !sameStrings(rec.names(), []string{"d3", "d2"}) {
				t.Errorf("calls = %v, want [d3 d2]: removal must stop at the first failure", rec.names())
			}
			if !sameStrings(res.Removed, []string{"d3"}) || !sameStrings(res.Failed, []string{"d2"}) || !res.Changed {
				t.Errorf("result = %+v", res)
			}
			l := loadLedger(t, dir) // Load validates the ledger
			if _, ok := l.Bundles["family"]; !ok {
				t.Error("family must stay recorded after a partial remove")
			}
			if _, ok := l.Plugins["d3"]; ok {
				t.Error("d3 is gone from disk but still in the ledger")
			}
			for _, n := range []string{"d1", "d2"} {
				if by := l.Plugins[n].InstalledBy; !sameStrings(by, []string{"family"}) {
					t.Errorf("%s installed_by = %v, want [family]", n, by)
				}
				if !isAlreadyInstalled(dir, n) {
					t.Errorf("%s was removed", n)
				}
			}

			// a rerun continues from there and forgets the bundle
			rec2 := &recorder{}
			opts2 := removeOpts(dir, &out, rec2)
			opts2.KeepData = true
			if _, err := Remove(ctx, "family", opts2); err != nil {
				t.Fatalf("rerun: %v", err)
			}
			if !sameStrings(rec2.names(), []string{"d2", "d1"}) {
				t.Errorf("rerun calls = %v, want [d2 d1]", rec2.names())
			}
			l = loadLedger(t, dir)
			if len(l.Bundles) != 0 || len(l.Plugins) != 0 {
				t.Errorf("ledger after the rerun = %+v, want empty", l)
			}
		})
	})
}

// TestRemoveUnrecordedBundle (R3): the ledger has no record of the bundle.
func TestRemoveUnrecordedBundle(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		useOverlapBundles(t)
		dir := t.TempDir()
		installPlugins(t, dir, "d1") // family is only partly installed: bootstrap does not record it
		var out bytes.Buffer
		rec := &recorder{}
		opts := removeOpts(dir, &out, rec)
		opts.KeepData = true
		_, err := Remove(context.Background(), "family", opts)
		if len(rec.calls) != 0 || !isAlreadyInstalled(dir, "d1") {
			t.Fatalf("nothing may be removed: calls=%v", rec.calls)
		}
		if compat.V15() {
			if d := errs.Describe(err); d == nil || d.Code != "E134" || errs.ExitCodeFor(err) != 1 {
				t.Errorf("v1.5: err = %v, want E134 exit 1", err)
			}
		} else if err != nil || !strings.Contains(out.String(), "not recorded") {
			t.Errorf("v1.4: err=%v out=%s, want a warning and exit 0", err, out.String())
		}
		if _, statErr := os.Stat(filepath.Join(dir, ".nself-state", "bundles.json")); !os.IsNotExist(statErr) {
			t.Errorf("an unrecorded remove wrote a ledger: %v", statErr)
		}
	})
}
