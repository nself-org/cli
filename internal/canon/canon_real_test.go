// Tests for the populated canon fragments (P7-REG-04, split by P7-CANON-02).
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
	"path/filepath"
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

// destructiveNamed reports whether any name token (split on spaces and
// hyphens) is a destructive verb, so `restore-drill` counts as restore.
func destructiveNamed(key string) bool {
	for _, tok := range strings.FieldsFunc(key, func(r rune) bool { return r == ' ' || r == '-' }) {
		switch tok {
		case "delete", "destroy", "drop", "prune", "purge", "reset", "clean", "wipe", "restore", "rollback", "revoke", "clear", "uninstall", "remove", "rm":
			return true
		}
	}
	return false
}

func load(t *testing.T) *canon.File {
	t.Helper()
	f, err := canon.LoadRaw()
	if err != nil {
		t.Fatalf("LoadRaw: %v", err)
	}
	return f
}

// fragmentKeys returns, per fragment file (lexical order), the command keys in
// file order, and the head comment of every key across all fragments.
func fragmentKeys(t *testing.T) (files []string, keys map[string][]string, head map[string]string) {
	t.Helper()
	names, err := filepath.Glob("domains/*.yaml")
	if err != nil || len(names) == 0 {
		t.Fatalf("no fragments under domains/: %v", err)
	}
	sort.Strings(names)
	keys = map[string][]string{}
	head = map[string]string{}
	for _, name := range names {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var doc yaml.Node
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		top := doc.Content[0]
		for i := 0; i+1 < len(top.Content); i += 2 {
			if top.Content[i].Value != "commands" {
				continue
			}
			cmds := top.Content[i+1]
			for j := 0; j+1 < len(cmds.Content); j += 2 {
				k := cmds.Content[j]
				keys[name] = append(keys[name], k.Value)
				head[k.Value] = strings.TrimSpace(k.HeadComment)
			}
		}
		files = append(files, name)
	}
	return files, keys, head
}

// comments returns every key (fragment order) and the head comment of each.
func comments(t *testing.T) (keys []string, head map[string]string) {
	t.Helper()
	files, per, head := fragmentKeys(t)
	for _, n := range files {
		keys = append(keys, per[n]...)
	}
	return keys, head
}

func TestPopulatedAndValid(t *testing.T) {
	f := load(t)
	if len(f.Commands) < 300 {
		t.Fatalf("the canon fragments hold %d entries; the live tree has more than 300 commands", len(f.Commands))
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
	files, per, _ := fragmentKeys(t)
	seen := map[string]string{}
	for _, f := range files {
		if !sort.StringsAreSorted(per[f]) {
			t.Errorf("%s: commands keys are not sorted; keep each fragment sorted by key", f)
		}
		for _, k := range per[f] {
			if prev, dup := seen[k]; dup {
				t.Errorf("duplicate key %q in %s and %s", k, prev, f)
			}
			seen[k] = f
		}
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
		t.Fatal("the canon has no help entry")
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
		if !destructiveNamed(key) || key == "help" {
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

// TestNoPersistentStateCommentImpliesRead: a comment that claims a command
// changes no persistent state must sit on a read entry (review S2).
func TestNoPersistentStateCommentImpliesRead(t *testing.T) {
	f := load(t)
	_, head := comments(t)
	for key, c := range head {
		if strings.Contains(c, "no persistent state") && f.Commands[key].SideEffect != canon.SideEffectRead {
			t.Errorf("%q: comment says no persistent state but the class is %s", key, f.Commands[key].SideEffect)
		}
	}
}

// TestCIServeIsRemote: ci serve runs untrusted webhook content and posts GitHub
// commit statuses, so it is at least the class of `ci`, and running the gate
// on the host (--allow-unsandboxed) escalates to destructive.
func TestCIServeIsRemote(t *testing.T) {
	f := load(t)
	serve, ci := f.Commands["ci serve"], f.Commands["ci"]
	if canon.Rank(serve.SideEffect) < canon.Rank(ci.SideEffect) || serve.SideEffect != canon.SideEffectRemote {
		t.Errorf("ci serve = %s, want remote (ci is %s)", serve.SideEffect, ci.SideEffect)
	}
	if serve.Flags["allow-unsandboxed"].SideEffect != canon.SideEffectDestructive {
		t.Errorf("ci serve --allow-unsandboxed = %+v, want side_effect destructive", serve.Flags["allow-unsandboxed"])
	}
	if serve.Output != canon.OutputStream {
		t.Errorf("ci serve output = %q, want stream", serve.Output)
	}
}

// TestSecuritySetupApplyIsDestructive: --apply edits sshd_config without a
// backup and enables the firewall; re-running does not undo a lock-out.
func TestSecuritySetupApplyIsDestructive(t *testing.T) {
	e := load(t).Commands["deploy security setup"]
	if e.SideEffect != canon.SideEffectRead || e.Flags["apply"].SideEffect != canon.SideEffectDestructive {
		t.Errorf("security setup = %+v, want read with --apply destructive", e)
	}
}
