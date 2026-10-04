// Tests for the populated canon.yaml (P7-REG-04).
//
// Purpose:     guard the declared command canon against the mistakes that make
//
//	the registry unsafe: an unclassified destructive command, a
//	class that departs from the name heuristic without a recorded
//	reason, an unsorted file, a plugin or envelope claim.
//
// Constraints: package canon_test; imports internal/canon, stdlib and yaml.v3
//
//	only (internal packages never import cmd/, Epic D2). Checks that
//	need the cobra tree (completeness, core == verbs present) run in
//	the verify script of P7-REG-04 and in the P7-REG-05/07 tests.
package canon_test

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"gopkg.in/yaml.v3"
)

// heuristic mirrors the P7-REG-04 name heuristic, keyed on the last word of a
// command path. A class that differs from it needs a `# file:line reason`
// comment above the entry.
func heuristic(key string) string {
	words := strings.Fields(key)
	last := words[len(words)-1]
	in := func(list string) bool {
		for _, w := range strings.Fields(list) {
			if w == last {
				return true
			}
		}
		return false
	}
	switch {
	case in("list show get status info search validate check verify count outdated logs tail urls version diff plan explain inspect"):
		return canon.SideEffectRead
	case in("delete destroy drop prune purge reset clean wipe restore rollback revoke clear uninstall remove rm"):
		return canon.SideEffectDestructive
	case in("deploy promote provision grant publish push sync release resize register"):
		return canon.SideEffectRemote
	}
	return canon.SideEffectWrite
}

func load(t *testing.T) *canon.File {
	t.Helper()
	f, err := canon.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return f
}

// comments returns the head comment of every key under `commands`, in file order.
func comments(t *testing.T) (keys []string, head map[string]string) {
	t.Helper()
	raw, err := os.ReadFile("canon.yaml")
	if err != nil {
		t.Fatalf("read canon.yaml: %v", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse canon.yaml: %v", err)
	}
	top := doc.Content[0]
	head = map[string]string{}
	for i := 0; i+1 < len(top.Content); i += 2 {
		if top.Content[i].Value != "commands" {
			continue
		}
		cmds := top.Content[i+1]
		for j := 0; j+1 < len(cmds.Content); j += 2 {
			k := cmds.Content[j]
			keys = append(keys, k.Value)
			head[k.Value] = strings.TrimSpace(k.HeadComment)
		}
	}
	return keys, head
}

func TestPopulatedAndValid(t *testing.T) {
	f := load(t)
	if len(f.Commands) < 300 {
		t.Fatalf("canon.yaml has %d entries; the live tree has more than 300 commands", len(f.Commands))
	}
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	for key, e := range f.Commands {
		if e.SideEffect == "" {
			t.Errorf("%q: side_effect is required on every entry", key)
		}
		if e.JSON == canon.JSONEnvelope {
			t.Errorf("%q: json envelope is derived, never declared", key)
		}
		if e.Canon == canon.CanonPlugin {
			t.Errorf("%q: no cobra-native command may be canon plugin", key)
		}
	}
}

func TestKeysSortedAndUnique(t *testing.T) {
	keys, _ := comments(t)
	if !sort.StringsAreSorted(keys) {
		t.Error("commands keys are not sorted; keep canon.yaml sorted by key")
	}
	seen := map[string]bool{}
	for _, k := range keys {
		if seen[k] {
			t.Errorf("duplicate key %q", k)
		}
		seen[k] = true
	}
}

func TestCoreEntriesAreVerbs(t *testing.T) {
	f := load(t)
	verbs := map[string]bool{}
	for _, v := range f.Verbs {
		verbs[v] = true
	}
	for key, e := range f.Commands {
		if e.Canon == canon.CanonCore && !verbs[key] {
			t.Errorf("%q is core but not an ADR 0016 verb", key)
		}
		if verbs[key] && e.Canon != canon.CanonCore {
			t.Errorf("verb %q must be core, got %q", key, e.Canon)
		}
	}
}

func TestDepthRules(t *testing.T) {
	f := load(t)
	for key, e := range f.Commands {
		if canon.Depth(key) >= 2 && e.Canon != "" && e.Canon != canon.CanonShim {
			t.Errorf("%q: depth >= 2 omits canon (subcommand) unless it is a deprecated-shim", key)
		}
		if canon.Depth(key) == 1 {
			switch e.Canon {
			case canon.CanonCore, canon.CanonPending, canon.CanonShim, canon.CanonBuiltin:
			default:
				t.Errorf("%q: top-level canon %q is not core, pending, deprecated-shim or builtin", key, e.Canon)
			}
		}
		if e.Canon == canon.CanonShim {
			tgt, ok := f.Commands[e.Target]
			if !ok {
				t.Errorf("%q: shim target %q has no entry", key, e.Target)
			} else if tgt.Canon == canon.CanonShim {
				t.Errorf("%q: shim target %q is itself a shim", key, e.Target)
			}
		}
	}
}

func TestHelpIsBuiltin(t *testing.T) {
	e, ok := load(t).Commands["help"]
	if !ok {
		t.Fatal("canon.yaml has no help entry")
	}
	if e.Canon != canon.CanonBuiltin || e.SideEffect != canon.SideEffectRead {
		t.Errorf("help = %+v, want {canon: builtin, side_effect: read}", e)
	}
}

// TestDestructiveNamedIsClassified: a command named like a destructive verb is
// remote or destructive, or carries a justification comment.
func TestDestructiveNamedIsClassified(t *testing.T) {
	f := load(t)
	_, head := comments(t)
	for key, e := range f.Commands {
		if heuristic(key) != canon.SideEffectDestructive || key == "help" {
			continue
		}
		if canon.Rank(e.SideEffect) >= canon.Rank(canon.SideEffectRemote) {
			continue
		}
		if !strings.Contains(head[key], ".go:") {
			t.Errorf("%q is destructive-named but classified %s without a file:line comment", key, e.SideEffect)
		}
	}
}

// TestDeviationsCarryEvidence: every class that differs from the name heuristic
// has a `# file:line reason` comment above its entry.
func TestDeviationsCarryEvidence(t *testing.T) {
	f := load(t)
	_, head := comments(t)
	for key, e := range f.Commands {
		if key == "help" || e.SideEffect == heuristic(key) {
			continue
		}
		if !strings.Contains(head[key], ".go:") {
			t.Errorf("%q: %s differs from the name heuristic (%s) and has no file:line comment", key, e.SideEffect, heuristic(key))
		}
	}
}

func TestDoctorOverrides(t *testing.T) {
	d := load(t).Commands["doctor"]
	want := map[string]canon.FlagOverride{
		"fix":           {SideEffect: canon.SideEffectWrite},
		"ai":            {SideEffect: canon.SideEffectWrite, JSON: canon.JSONLegacy},
		"install-check": {JSON: canon.JSONLegacy},
	}
	if len(d.Flags) != len(want) {
		t.Fatalf("doctor flags = %v, want exactly %v", d.Flags, want)
	}
	for name, w := range want {
		if d.Flags[name] != w {
			t.Errorf("doctor --%s = %+v, want %+v", name, d.Flags[name], w)
		}
	}
}

func TestStateExitCodes(t *testing.T) {
	f := load(t)
	for key, want := range map[string]map[string]string{
		"status": {"1": "one or more services are not yet healthy (starting)", "2": "one or more services are unhealthy"},
		"doctor": {"1": "one or more checks failed", "2": "warnings only, no failures"},
	} {
		got := f.Commands[key].ExitCodes
		for code, text := range want {
			if got[code] != text {
				t.Errorf("%s exit_codes[%s] = %q, want %q", key, code, got[code], text)
			}
		}
	}
	if len(f.Commands["status"].ExitCodesV15) == 0 || len(f.Commands["doctor"].ExitCodesV15) == 0 {
		t.Error("status and doctor must document the v1.5 state codes (D10)")
	}
}

func TestStreamCommands(t *testing.T) {
	f := load(t)
	for _, key := range []string{"logs", "license tail", "mcp"} {
		if f.Commands[key].Output != canon.OutputStream {
			t.Errorf("%q must be output stream", key)
		}
	}
	for key, e := range f.Commands {
		w := strings.Fields(key)
		switch w[len(w)-1] {
		case "serve", "watch", "tail":
			if e.Output != canon.OutputStream {
				t.Errorf("%q is a serve/watch/tail leaf and must be output stream", key)
			}
		}
	}
}
