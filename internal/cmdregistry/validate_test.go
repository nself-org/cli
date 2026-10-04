package cmdregistry

import (
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
)

// buildWith builds the fixture tree with mutated canon data and data types.
func buildWith(t *testing.T, mutate func(f *canon.File, types map[string]any)) error {
	t.Helper()
	f := fixtureCanon(t)
	types := fixtureTypes()
	mutate(f, types)
	_, err := Build(fixtureRoot(false), f, types, fixtureOpts(true))
	return err
}

func setEntry(f *canon.File, key string, fn func(e *canon.Entry)) {
	e := f.Commands[key]
	fn(&e)
	f.Commands[key] = e
}

// TestValidationRules has one failing fixture per Epic validation rule; every
// error must name the offending path.
func TestValidationRules(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(f *canon.File, types map[string]any)
		wantPath string
		wantMsg  string
	}{
		{"command without entry", func(f *canon.File, _ map[string]any) { delete(f.Commands, "hub list") }, `commands["hub list"]`, "no canon entry"},
		{"entry without command", func(f *canon.File, _ map[string]any) { f.Commands["ghost cmd"] = canon.Entry{SideEffect: "read"} }, `commands["ghost cmd"]`, "does not resolve"},
		{"enum invalid", func(f *canon.File, _ map[string]any) {
			setEntry(f, "add", func(e *canon.Entry) { e.SideEffect = "explosive" })
		}, `commands["add"]`, "side_effect"},
		{"depth rule", func(f *canon.File, _ map[string]any) {
			setEntry(f, "hub get", func(e *canon.Entry) { e.Canon = "pending" })
		}, `commands["hub get"]`, "depth >= 2"},
		{"core iff in verbs", func(f *canon.File, _ map[string]any) {
			setEntry(f, "deploy", func(e *canon.Entry) { e.Canon = "core" })
		}, `commands["deploy"]`, "not in verbs"},
		{"verbs ceiling", func(f *canon.File, _ map[string]any) {
			for i := 0; i < 20; i++ {
				f.Verbs = append(f.Verbs, "extra"+string(rune('a'+i)))
			}
		}, "verbs", "ceiling of 20"},
		{"plugin rejected", func(f *canon.File, _ map[string]any) {
			setEntry(f, "hub list", func(e *canon.Entry) { e.Canon = "plugin" })
		}, `commands["hub list"]`, "rejected on cobra-native"},
		{"shim requires target", func(f *canon.File, _ map[string]any) {
			setEntry(f, "oldstatus", func(e *canon.Entry) { e.Target = "" })
		}, `commands["oldstatus"]`, "requires target"},
		{"target unresolved", func(f *canon.File, _ map[string]any) {
			setEntry(f, "oldstatus", func(e *canon.Entry) { e.Target = "nope" })
		}, `commands["oldstatus"]`, "does not resolve"},
		{"target is a shim", func(f *canon.File, _ map[string]any) {
			f.Commands["hub list"] = canon.Entry{Canon: "deprecated-shim", Target: "oldstatus", SideEffect: "read"}
		}, `commands["hub list"]`, "itself a deprecated-shim"},
		{"target on non-shim", func(f *canon.File, _ map[string]any) {
			setEntry(f, "add", func(e *canon.Entry) { e.Target = "status" })
		}, `commands["add"]`, "only allowed on a deprecated-shim"},
		{"side_effect required when runnable", func(f *canon.File, _ map[string]any) {
			setEntry(f, "status", func(e *canon.Entry) { e.SideEffect = "" })
		}, `commands["status"]`, "side_effect is required"},
		{"flag override names a missing flag", func(f *canon.File, _ map[string]any) {
			setEntry(f, "doctor", func(e *canon.Entry) { e.Flags["nope"] = canon.FlagOverride{SideEffect: "write"} })
		}, `commands["doctor"] flag "nope"`, "no such flag"},
		{"flag override must escalate", func(f *canon.File, _ map[string]any) {
			setEntry(f, "doctor", func(e *canon.Entry) { e.Flags["fix"] = canon.FlagOverride{SideEffect: "read"} })
		}, `commands["doctor"] flag "fix"`, "must outrank"},
		{"flag override cannot lower", func(f *canon.File, _ map[string]any) {
			setEntry(f, "deploy", func(e *canon.Entry) { e.Flags["stream"] = canon.FlagOverride{SideEffect: "write"} })
		}, `commands["deploy"] flag "stream"`, "must outrank"},
		{"flag output stream on a stream command", func(f *canon.File, _ map[string]any) {
			setEntry(f, "logs", func(e *canon.Entry) { e.Flags = map[string]canon.FlagOverride{"follow": {Output: "stream"}} })
		}, `commands["logs"] flag "follow"`, "only allowed on a command whose output is document"},
		{"flag output not stream", func(f *canon.File, _ map[string]any) {
			setEntry(f, "deploy", func(e *canon.Entry) { e.Flags["stream"] = canon.FlagOverride{Output: "interactive"} })
		}, `commands["deploy"] flag "stream"`, "only flag-level value is stream"},
		{"envelope declared in YAML", func(f *canon.File, _ map[string]any) {
			setEntry(f, "add", func(e *canon.Entry) { e.JSON = "envelope" })
		}, `commands["add"]`, "may not be declared in YAML"},
		{"registered type conflicts with legacy", func(_ *canon.File, types map[string]any) { types["logs"] = struct{}{} }, `commands["logs"]`, "json: legacy"},
		{"registered type without command", func(_ *canon.File, types map[string]any) { types["nope"] = struct{}{} }, `dataTypes["nope"]`, "does not resolve"},
		{"exit code key format", func(f *canon.File, _ map[string]any) {
			setEntry(f, "status", func(e *canon.Entry) { e.ExitCodes["two"] = "x" })
		}, `commands["status"] exit_codes`, "integer 0-125"},
		{"exit code v15 range", func(f *canon.File, _ map[string]any) {
			setEntry(f, "status", func(e *canon.Entry) { e.ExitCodesV15["200"] = "x" })
		}, `commands["status"] exit_codes_v15`, "integer 0-125"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := buildWith(t, c.mutate)
			if err == nil {
				t.Fatal("Build accepted an invalid input")
			}
			var ve *canon.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("want *canon.ValidationError, got %T", err)
			}
			if msg := err.Error(); !strings.Contains(msg, c.wantPath) || !strings.Contains(msg, c.wantMsg) {
				t.Errorf("error does not name %q with %q:\n%s", c.wantPath, c.wantMsg, msg)
			}
		})
	}
}

func TestBuildAggregatesEveryProblem(t *testing.T) {
	err := buildWith(t, func(f *canon.File, _ map[string]any) {
		delete(f.Commands, "hub list")
		f.Commands["ghost cmd"] = canon.Entry{SideEffect: "read"}
		setEntry(f, "status", func(e *canon.Entry) { e.SideEffect = "" })
	})
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{`commands["hub list"]`, `commands["ghost cmd"]`, `commands["status"]`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("aggregated error misses %s:\n%s", want, err)
		}
	}
}

func TestBuildRejectsNilInputs(t *testing.T) {
	if _, err := Build(nil, fixtureCanon(t), nil, BuildOptions{}); err == nil {
		t.Error("nil root accepted")
	}
	if _, err := Build(fixtureRoot(false), nil, nil, BuildOptions{}); err == nil {
		t.Error("nil canon accepted")
	}
}
