package cmdregistry

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/spf13/cobra"
)

func TestEffectiveJSON(t *testing.T) {
	c := &Command{JSON: canon.JSONEnvelope, Flags: []Flag{
		{Name: "plain"},
		{Name: "raw", JSON: strPtr(canon.JSONLegacy)},
		{Name: "quiet", JSON: strPtr(canon.JSONNone)},
	}}
	for _, tc := range []struct {
		name string
		set  map[string]bool
		want string
	}{
		{"no flag set keeps the command value", nil, canon.JSONEnvelope},
		{"flag without override keeps the command value", map[string]bool{"plain": true}, canon.JSONEnvelope},
		{"unset override flag is ignored", map[string]bool{"plain": true, "raw": false}, canon.JSONEnvelope},
		{"legacy override replaces envelope", map[string]bool{"raw": true}, canon.JSONLegacy},
		{"none override replaces envelope", map[string]bool{"quiet": true}, canon.JSONNone},
		{"none wins over legacy", map[string]bool{"raw": true, "quiet": true}, canon.JSONNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := EffectiveJSON(c, tc.set); got != tc.want {
				t.Fatalf("EffectiveJSON = %q, want %q", got, tc.want)
			}
		})
	}
	if got := EffectiveJSON(nil, map[string]bool{"raw": true}); got != "" {
		t.Fatalf("nil command has effective json %q", got)
	}
}

func TestEffectiveJSONFlagCannotPromoteToEnvelope(t *testing.T) {
	for _, base := range []string{canon.JSONLegacy, canon.JSONNone} {
		c := &Command{JSON: base, Flags: []Flag{{Name: "env", JSON: strPtr(canon.JSONEnvelope)}}}
		if got := EffectiveJSON(c, map[string]bool{"env": true}); got != base {
			t.Fatalf("base %q with envelope flag = %q, want %q", base, got, base)
		}
	}
}

func TestBracketedUnclosed(t *testing.T) {
	for _, tc := range []struct {
		in, wantTok, wantRest string
	}{
		{"<file", "file", ""},
		{"[opt [nested]", "opt [nested]", ""},
		{"[a [b]] tail", "a [b]", " tail"},
	} {
		open, closeCh := tc.in[0], byte('>')
		if open == '[' {
			closeCh = ']'
		}
		tok, rest := bracketed(tc.in, open, closeCh)
		if tok != tc.wantTok || rest != tc.wantRest {
			t.Errorf("bracketed(%q) = (%q, %q), want (%q, %q)", tc.in, tok, rest, tc.wantTok, tc.wantRest)
		}
	}
	want := []Arg{{Name: "file", Required: true}}
	if got := parseArgs("get <file"); !reflect.DeepEqual(got, want) {
		t.Errorf("parseArgs with an unclosed bracket = %v, want %v", got, want)
	}
}

func TestApplyFlagEnvNilRegistry(t *testing.T) {
	err := ApplyFlagEnv(nil, map[string]map[string]string{"nself": {"x": "X"}}, nil, true)
	if err == nil || !strings.Contains(err.Error(), "needs a registry") {
		t.Fatalf("nil registry accepted: %v", err)
	}
}

func TestBuildNeedsRootAndCanon(t *testing.T) {
	if _, err := Build(nil, fixtureCanon(t), nil, BuildOptions{}); err == nil || !strings.Contains(err.Error(), "needs a root command") {
		t.Fatalf("nil root accepted: %v", err)
	}
	if _, err := Build(fixtureRoot(false), nil, nil, BuildOptions{}); err == nil || !strings.Contains(err.Error(), "needs a root command") {
		t.Fatalf("nil canon accepted: %v", err)
	}
}

func TestBuildV15OnlyEnvelopeMustResolve(t *testing.T) {
	opts := fixtureOpts(true)
	opts.V15OnlyEnvelope = map[string]bool{"ghost": true}
	_, err := Build(fixtureRoot(false), fixtureCanon(t), fixtureTypes(), opts)
	if err == nil || !strings.Contains(err.Error(), `V15OnlyEnvelope["ghost"]`) {
		t.Fatalf("unresolved V15OnlyEnvelope path accepted: %v", err)
	}
}

func TestBuiltinMountKeepsPresplitCanonInV14(t *testing.T) {
	root := fixtureRoot(false)
	root.AddCommand(&cobra.Command{Use: "fixture", RunE: noop, Annotations: map[string]string{annPluginKey: "fixture", annSourceKey: sourceBuiltin}})
	f := fixtureCanon(t)
	f.Commands["fixture"] = canon.Entry{Canon: canon.CanonPending, SideEffect: canon.SideEffectRead}
	r, err := Build(root, f, fixtureTypes(), fixtureOpts(false))
	if err != nil {
		t.Fatal(err)
	}
	c, ok := r.Lookup("nself fixture")
	if !ok || c.Canon != canon.CanonPending || c.Plugin != nil {
		t.Fatalf("v1.4 builtin family must keep its canon classification and have no plugin identity: %+v, found=%t", c, ok)
	}
}

func TestValidateConfirmProblems(t *testing.T) {
	cmd := &cobra.Command{Use: "reset", RunE: noop}
	cmd.Flags().Bool("yes", false, "")
	declared := localFlags(cmd)
	for _, tc := range []struct {
		name     string
		src      *canon.Confirm
		runnable bool
		want     string
	}{
		{"not runnable", &canon.Confirm{Flags: []string{"yes"}}, false, "command is not runnable"},
		{"duplicate flag", &canon.Confirm{Flags: []string{"yes", "yes"}}, true, "empty or duplicated"},
		{"empty flag", &canon.Confirm{Flags: []string{""}}, true, "empty or duplicated"},
		{"plan without flags", &canon.Confirm{Flags: []string{"yes"}, Plan: &canon.PlanForm{}}, true, "flag and id_flag are required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			confirm, problems := validateConfirm("reset", tc.src, tc.runnable, cmd, declared)
			if confirm != nil {
				t.Fatalf("invalid confirm produced a value: %+v", confirm)
			}
			if len(problems) == 0 || !strings.Contains(strings.Join(problems, "\n"), tc.want) {
				t.Fatalf("want a problem containing %q, got %v", tc.want, problems)
			}
		})
	}
}

// captureStderr runs fn with os.Stderr redirected and returns what it wrote.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()
	fn()
	w.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestWarnPluginOncePerSlug(t *testing.T) {
	warned := map[string]bool{}
	out := captureStderr(t, func() {
		warnPlugin("demo", warned, "invalid surface")
		warnPlugin("demo", warned, "invalid surface")
		warnPlugin("other", warned, "invalid confirm JSON")
	})
	if got := strings.Count(out, "\n"); got != 2 {
		t.Fatalf("want one warning per slug (2 lines), got %d: %q", got, out)
	}
	if !strings.Contains(out, "[E437] plugin demo: invalid surface; confirm disabled") || !strings.Contains(out, "plugin other: invalid confirm JSON") {
		t.Fatalf("unexpected warning text: %q", out)
	}
}

func TestPluginInvalidConfirmJSONIsDropped(t *testing.T) {
	root := pluginFixture()
	cmd, _, err := root.Find([]string{"demo", "sub"})
	if err != nil {
		t.Fatal(err)
	}
	cmd.Annotations[annConfirmKey] = `{not json`
	var r *Registry
	out := captureStderr(t, func() { r = mustBuild(t, root, true) })
	c, ok := r.Lookup("nself demo sub")
	if !ok || c.Confirm != nil {
		t.Fatalf("invalid confirm JSON must not create a confirm: %+v, found=%t", c, ok)
	}
	if !strings.Contains(out, "invalid confirm JSON") {
		t.Fatalf("no warning for invalid confirm JSON: %q", out)
	}
}
