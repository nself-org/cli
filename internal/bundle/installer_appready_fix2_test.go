package bundle

// installer_appready_fix2_test.go — P7-PLUG-19 fix round 2: E135 always exits 2
// (M1), a free bundle is not removable as a unit (M2) and members that were on
// disk before the install are recorded explicit (M3).

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle/appready"
	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// TestE135ExitsTwoEvenWithProcessErrorInChain: a failing start step wraps a real
// *exec.ExitError (docker compose exit 7). E135 must still exit 2 and the cause
// must stay reachable through errors.As.
func TestE135ExitsTwoEvenWithProcessErrorInChain(t *testing.T) {
	compattest.Set(t, true)
	cause := exec.Command("sh", "-c", "exit 7").Run()
	var ee *exec.ExitError
	if !errors.As(cause, &ee) || ee.ExitCode() != 7 {
		t.Fatalf("fixture: want a real exec.ExitError with code 7, got %v", cause)
	}
	f := &appready.Failure{Step: appready.StepStart, Plugin: "jobs", Err: fmt.Errorf("compose up: %w", cause)}
	err := appReadyVerdict(&bytes.Buffer{}, "task", f)
	if err == nil {
		t.Fatal("v1.5 must return E135")
	}
	if got := errs.ExitCodeFor(err); got != 2 {
		t.Errorf("ExitCodeFor = %d, want 2 (not the docker status 7)", got)
	}
	if d := errs.Describe(err); d == nil || d.Code != "E135" {
		t.Errorf("detail = %+v, want E135", d)
	}
	var reached *exec.ExitError
	if !errors.As(err, &reached) || reached.ExitCode() != 7 {
		t.Errorf("the process error is no longer reachable through errors.As: %v", err)
	}
}

// TestRemoveRefusesFreeBundle: the Task Bundle is installable but not removable
// as a unit until P7-PLUG-20, in both modes, and no remover hook is called.
func TestRemoveRefusesFreeBundle(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		pluginDir := isolate(t)
		writeMember(t, pluginDir, "notifications", 1, nil)
		var calls int
		res, err := Remove(context.Background(), "task", RemoveOpts{
			Out: &bytes.Buffer{}, PluginDir: pluginDir, Config: &config.Config{},
			remover: func(context.Context, *config.Config, string, string, bool, bool) error { calls++; return nil },
		})
		if err == nil || !strings.Contains(err.Error(), "not installable/removable as a unit (meta or free)") {
			t.Fatalf("err = %v, want the free-bundle refusal", err)
		}
		if res != nil || calls != 0 {
			t.Errorf("a refused remove must change nothing: result=%v calls=%d", res, calls)
		}
		if !isAlreadyInstalled(pluginDir, "notifications") {
			t.Error("the member plugin was removed")
		}
	})
}

// TestBundleInstallRecordsPreexistingMembersExplicit: x was on disk before the
// install, the ledger exists and has no record of it, so x was installed on its
// own: it ends explicit with installed_by [task]. jobs, installed now, is not.
func TestBundleInstallRecordsPreexistingMembersExplicit(t *testing.T) {
	pluginDir := isolate(t)
	setupOfflineRegistry(t, mockTaskRegistry())
	store := newLedgerStore(pluginDir)
	if err := store.Update(func(l *ledger.Ledger) error {
		l.MarkExplicit(ledger.InstalledPlugin{Name: "other", Version: "1.0.0", Tier: ledger.TierFree})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	writeMember(t, pluginDir, "notifications", 1, nil) // on disk after the ledger exists, unrecorded
	var out bytes.Buffer
	opts := freeOpts(t, pluginDir, &out, func(name string) error { writeMember(t, pluginDir, name, 1, nil); return nil })
	if _, err := Install(context.Background(), "task", opts); err != nil {
		t.Fatalf("install: %v\n%s", err, out.String())
	}
	l, _, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	n := l.Plugins["notifications"]
	if !n.Explicit || len(n.InstalledBy) != 1 || n.InstalledBy[0] != "task" || n.Version != "1.0.0" {
		t.Errorf("notifications = %+v, want explicit:true installed_by [task]", n)
	}
	j := l.Plugins["jobs"]
	if j.Explicit || len(j.InstalledBy) != 1 || j.InstalledBy[0] != "task" {
		t.Errorf("jobs = %+v, want explicit:false installed_by [task]", j)
	}
	if !l.Plugins["other"].Explicit {
		t.Error("unrelated record lost")
	}
}
