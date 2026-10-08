package canon

// Tests for the mode views (P7-CANON-02, EPIC D3): fixture views, the round
// trip to the pre-move fixture, equivalence on the real data, and Load's
// mode awareness.

import (
	"os"
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
)

func movedRaw(t *testing.T) *File {
	t.Helper()
	f, err := LoadFS(movedFS(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestEffectiveViews(t *testing.T) {
	raw := movedRaw(t)
	v15, err := raw.View(true)
	if err != nil {
		t.Fatal(err)
	}
	v14, err := raw.View(false)
	if err != nil {
		t.Fatal(err)
	}

	// v1.5: canonical paths, generated shims, builtin, hub; mode v1.4 entries gone.
	for _, k := range []string{"config env", "config env use", "add", "db migrate up", "db migrate", "service", "stop"} {
		if _, ok := v15.Commands[k]; !ok {
			t.Errorf("v1.5 view lacks %q", k)
		}
	}
	for _, k := range []string{"plugin init", "plugin marketplace"} {
		if _, ok := v15.Commands[k]; ok {
			t.Errorf("v1.5 view includes mode v1.4 entry %q", k)
		}
	}
	shims := map[string]string{"env": "config env", "install": "add", "migrate up": "db migrate up", "migrate": "db", "service stop": "stop"}
	for from, to := range shims {
		e := v15.Commands[from]
		if e.Canon != CanonShim || e.Target != to {
			t.Errorf("v1.5 %q = %+v, want deprecated-shim -> %s", from, e, to)
		}
	}
	if got := v15.Commands["install"].SideEffect; got != SideEffectRemote {
		t.Errorf("shim side_effect = %q, want the target's (remote)", got)
	}
	if got := v15.Commands["completion"].Canon; got != CanonBuiltin {
		t.Errorf("v1.5 completion canon = %q, want builtin", got)
	}
	if e := v15.Commands["db migrate"]; !reflect.DeepEqual(e, Entry{}) {
		t.Errorf("hub entry = %+v, want the empty subcommand entry", e)
	}
	for _, e := range v15.Commands {
		if e.Mode != "" {
			t.Errorf("a view must not carry mode: %+v", e)
		}
	}

	// v1.4: moved entries back at their old paths, pre-move canon status.
	if e := v14.Commands["env"]; e.Canon != CanonPending || e.SideEffect != SideEffectRead {
		t.Errorf("v1.4 env = %+v, want pending read", e)
	}
	if e := v14.Commands["migrate up"]; e.Canon != "" {
		t.Errorf("v1.4 migrate up canon = %q, want subcommand (empty)", e.Canon)
	}
	for _, k := range []string{"env use", "install", "migrate", "service stop", "plugin init", "plugin marketplace"} {
		if _, ok := v14.Commands[k]; !ok {
			t.Errorf("v1.4 view lacks %q", k)
		}
	}
	for _, k := range []string{"config env", "add", "db migrate", "db migrate up"} {
		if _, ok := v14.Commands[k]; ok {
			t.Errorf("v1.4 view has canonical-only path %q", k)
		}
	}
	if len(v14.Hubs)+len(v14.Moves)+len(v15.Shims) != 0 {
		t.Error("views must not carry rows")
	}
}

// TestEffectiveRoundTrip: Effective(false) of the moved fixture equals the
// pre-move fixture exactly.
func TestEffectiveRoundTrip(t *testing.T) {
	b, err := os.ReadFile("testdata/fragments/premove.yaml")
	if err != nil {
		t.Fatal(err)
	}
	pre, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	v14, err := movedRaw(t).View(false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v14.Commands, pre.Commands) {
		t.Errorf("v1.4 view differs from pre-move fixture:\n got %v\nwant %v", v14.Commands, pre.Commands)
	}
	if !reflect.DeepEqual(v14.Verbs, pre.Verbs) || v14.SchemaVersion != pre.SchemaVersion {
		t.Errorf("header differs: %v/%d vs %v/%d", v14.Verbs, v14.SchemaVersion, pre.Verbs, pre.SchemaVersion)
	}
}

// TestEffectiveRemapsShimTargets: an authored shim whose target moved keeps
// pointing at the target's v1.4 path in the v1.4 view.
func TestEffectiveRemapsShimTargets(t *testing.T) {
	raw := movedRaw(t)
	raw.Commands["old-env"] = Entry{Canon: CanonShim, Target: "config env use", SideEffect: SideEffectWrite}
	v14, err := raw.View(false)
	if err != nil {
		t.Fatal(err)
	}
	if got := v14.Commands["old-env"].Target; got != "env use" {
		t.Errorf("v1.4 shim target = %q, want env use", got)
	}
	v15, _ := raw.View(true)
	if got := v15.Commands["old-env"].Target; got != "config env use" {
		t.Errorf("v1.5 shim target = %q, want config env use", got)
	}
}

// TestEffectivePresplitEquivalence: with the real fragments, both views equal
// the pre-split canon.yaml entry set (no move rows yet), or, once rows exist,
// the v1.4 view still contains every pre-split entry unchanged, except builtin
// classifications that preserve the command and its v1.4 behavior.
func TestEffectivePresplitEquivalence(t *testing.T) {
	pre := readPresplit(t)
	raw, err := LoadRaw()
	if err != nil {
		t.Fatal(err)
	}
	rows := len(raw.Hubs) + len(raw.Moves) + len(raw.Shims) + len(raw.RetiredHubs) + len(raw.Builtins) + len(raw.Breakouts) + len(raw.Removed)
	for _, v15 := range []bool{false, true} {
		v, err := Effective(v15)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(v.Verbs, pre.Verbs) {
			t.Errorf("v15=%v: verbs differ", v15)
		}
		if rows == 0 || !v15 {
			for k, want := range pre.Commands {
				if k == "completion" || k == "man" || k == "version" {
					want.Canon = CanonBuiltin
				}
				if got, ok := v.Commands[k]; !ok || !reflect.DeepEqual(got, want) {
					t.Errorf("v15=%v: %q = %+v (present %v), want %+v", v15, k, got, ok, want)
				}
			}
		}
		if rows == 0 && len(v.Commands) != len(pre.Commands) {
			t.Errorf("v15=%v: %d entries, pre-split had %d", v15, len(v.Commands), len(pre.Commands))
		}
	}
}

// TestLoadModeAware: Load returns the view of the running compat mode, memoised.
func TestLoadModeAware(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		want, err := Effective(compat.V15())
		if err != nil {
			t.Fatal(err)
		}
		got, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("Load() is not the memoised Effective(%v) view", compat.V15())
		}
		if again, _ := Load(); again != got {
			t.Error("Load is not memoised")
		}
	})
	v14, _ := Effective(false)
	v15, _ := Effective(true)
	if v14 == v15 {
		t.Error("the two modes must have separate views")
	}
	raw, _ := LoadRaw()
	if again, _ := LoadRaw(); again != raw {
		t.Error("LoadRaw is not memoised")
	}
}
