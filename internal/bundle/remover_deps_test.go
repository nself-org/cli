package bundle

// remover_deps_test.go — D-0325: a member that an installed plugin outside the
// plan depends on is kept and recorded explicit. Fixture: chat={px,py},
// claw={py,pz}; "outsider" is an installed plugin in no bundle.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/compat/compattest"
)

// setDeps rewrites the plugin.json of an installed plugin with dependencies.
func setDeps(t *testing.T, pluginDir, name string, deps ...string) {
	t.Helper()
	path := filepath.Join(pluginDir, name, "plugin.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m["dependencies"] = deps
	out, _ := json.Marshal(m)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

// chatDir installs px, py and outsider and records chat.
func chatDir(t *testing.T) string {
	t.Helper()
	useOverlapBundles(t)
	dir := t.TempDir()
	installPlugins(t, dir, "px", "py", "outsider")
	recordBundle(t, dir, "chat")
	return dir
}

func removeChat(t *testing.T, dir string, mod func(*RemoveOpts)) (*RemoveResult, string, *recorder, error) {
	t.Helper()
	var out bytes.Buffer
	rec := &recorder{}
	opts := baseOpts(dir, &out, rec)
	if mod != nil {
		mod(&opts)
	}
	res, err := Remove(context.Background(), "chat", opts)
	return res, out.String(), rec, err
}

func TestRemoveKeepsMemberAnOutsiderDependsOn(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		dir := chatDir(t)
		setDeps(t, dir, "outsider", "px")
		res, out, rec, err := removeChat(t, dir, nil)
		if err != nil {
			t.Fatalf("remove: %v\n%s", err, out)
		}
		if !sameStrings(rec.names(), []string{"py"}) || !sameStrings(res.Removed, []string{"py"}) {
			t.Errorf("removed %v, want only py", rec.names())
		}
		if !strings.Contains(out, "kept: px (needed by outsider; recorded explicit)") {
			t.Errorf("no keep line:\n%s", out)
		}
		if !isAlreadyInstalled(dir, "px") {
			t.Error("px was removed")
		}
		l := loadLedger(t, dir)
		px := l.Plugins["px"]
		if !px.Explicit || len(px.InstalledBy) != 0 || px.Version != "1.0.0" {
			t.Errorf("px record = %+v, want explicit, no installed_by, facts kept", px)
		}
		if _, ok := l.Bundles["chat"]; ok {
			t.Error("chat still recorded")
		}
		if err := l.Validate(); err != nil {
			t.Error(err)
		}
	})
}

// NEGATIVE CONTROL: without the dependency the member goes, so the test above
// is detecting the keep and not a general failure to remove.
func TestRemoveWithoutDependencyRemovesMember(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		dir := chatDir(t)
		_, out, rec, err := removeChat(t, dir, nil)
		if err != nil {
			t.Fatalf("remove: %v\n%s", err, out)
		}
		if !sameStrings(rec.names(), []string{"py", "px"}) {
			t.Errorf("removed %v, want [py px]", rec.names())
		}
		if strings.Contains(out, "needed by") {
			t.Errorf("unexpected keep:\n%s", out)
		}
	})
}

func TestRemoveKeepsTransitively(t *testing.T) {
	dir := chatDir(t)
	setDeps(t, dir, "outsider", "px") // outsider -> px -> py
	setDeps(t, dir, "px", "py")
	_, out, rec, err := removeChat(t, dir, nil)
	if err != nil || len(rec.calls) != 0 {
		t.Fatalf("err=%v removed=%v, want nothing removed\n%s", err, rec.names(), out)
	}
	for _, want := range []string{"kept: px (needed by outsider; recorded explicit)", "kept: py (needed by px; recorded explicit)"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	l := loadLedger(t, dir)
	if !l.Plugins["px"].Explicit || !l.Plugins["py"].Explicit {
		t.Errorf("both must be recorded explicit: %+v", l.Plugins)
	}
}

func TestRemoveMemberDependingOnMemberRemovesBoth(t *testing.T) {
	dir := chatDir(t)
	setDeps(t, dir, "px", "py") // both planned
	res, out, rec, err := removeChat(t, dir, nil)
	if err != nil || !sameStrings(res.Removed, []string{"py", "px"}) {
		t.Fatalf("err=%v removed=%v\n%s", err, res.Removed, out)
	}
	for _, c := range rec.calls {
		if !c.Force {
			t.Errorf("%s removed without force", c.Name)
		}
	}
}

func TestRemoveDependentIsAKeptSharedMember(t *testing.T) {
	// py stays (claw also lists it); py depends on px, so px stays too.
	useOverlapBundles(t)
	dir := t.TempDir()
	installPlugins(t, dir, "px", "py", "pz")
	recordBundle(t, dir, "chat")
	recordBundle(t, dir, "claw")
	setDeps(t, dir, "py", "px")
	_, out, rec, err := removeChat(t, dir, nil)
	if err != nil || len(rec.calls) != 0 {
		t.Fatalf("err=%v removed=%v\n%s", err, rec.names(), out)
	}
	if !strings.Contains(out, "kept: px (needed by py; recorded explicit)") {
		t.Errorf("missing keep line:\n%s", out)
	}
}

func TestRemoveDryRunShowsDependencyKeepAndWritesNothing(t *testing.T) {
	dir := chatDir(t)
	setDeps(t, dir, "outsider", "px")
	before := ledgerBytes(t, dir)
	res, out, rec, err := removeChat(t, dir, func(o *RemoveOpts) { o.DryRun = true })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "kept: px (needed by outsider; recorded explicit)") || !sameStrings(res.Planned, []string{"py"}) {
		t.Errorf("planned=%v out:\n%s", res.Planned, out)
	}
	if len(rec.calls) != 0 || !bytes.Equal(before, ledgerBytes(t, dir)) || intentFileExists(dir) {
		t.Error("a dry run changed state")
	}
}

func TestRemoveScanErrorRefusesThePlan(t *testing.T) {
	dir := chatDir(t)
	before := ledgerBytes(t, dir)
	_, _, rec, err := removeChat(t, dir, func(o *RemoveOpts) {
		o.reverseDeps = func(string, string) ([]string, error) { return nil, errors.New("scan broke") }
	})
	if err == nil || !strings.Contains(err.Error(), "scan broke") {
		t.Fatalf("err = %v, want the scan error", err)
	}
	if len(rec.calls) != 0 || !bytes.Equal(before, ledgerBytes(t, dir)) || intentFileExists(dir) {
		t.Error("a refused plan changed state")
	}
}

func TestKeepDependedOnTable(t *testing.T) {
	deps := map[string][]string{
		"a": {"b"},       // a needed by b
		"b": {"outside"}, // b needed by something outside
		"c": {"a"},       // c needed by a
	}
	cases := []struct {
		name    string
		planned []string
		want    []string
	}{
		{"chain keeps all", []string{"c", "a", "b"}, nil},
		{"only b outside", []string{"a", "b"}, nil},
		{"unrelated removed", []string{"z"}, []string{"z"}},
	}
	scan := func(_, name string) ([]string, error) { return deps[name], nil }
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan := removePlan{Planned: c.planned, work: ledger.New()}
			if err := keepDependedOn(&plan, ledger.New(), t.TempDir(), scan); err != nil {
				t.Fatal(err)
			}
			if !sameStrings(plan.Planned, c.want) {
				t.Errorf("planned = %v, want %v", plan.Planned, c.want)
			}
			if len(plan.Planned)+len(plan.Kept) != len(c.planned) {
				t.Errorf("planned %v + kept %v lost a member", plan.Planned, plan.Kept)
			}
		})
	}
}
