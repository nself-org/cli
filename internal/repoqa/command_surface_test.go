package repoqa

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
)

// surfaceBudgetFile holds the number of visible top-level commands the v1.5
// core binary registers: the v1.5 canon of ADR 0016 (at most 20, one per canon
// verb). Since P7-CANON-17 it equals reality, so a new top-level command and a
// removed one both fail until the file is edited on purpose; the canon verb
// list in internal/canon is the permanent ceiling (TestCanonCeiling).
const surfaceBudgetFile = ".github/command-surface-budget.txt"

func readSurfaceBudget(t *testing.T, root string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, surfaceBudgetFile))
	if err != nil {
		t.Fatalf("read %s: %v", surfaceBudgetFile, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			t.Fatalf("%s: %q is not an integer", surfaceBudgetFile, line)
		}
		return n
	}
	t.Fatalf("%s contains no budget number", surfaceBudgetFile)
	return 0
}

// topLevelCommandNames reads the generated inventory rather than importing
// cmd/commands, which would make this package depend on the whole CLI.
func topLevelCommandNames(t *testing.T, root string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".github", "command-inventory.json"))
	if err != nil {
		t.Fatalf("read command inventory: %v (run `make cmd-inventory`)", err)
	}
	var entries []inventoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("parse command inventory: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name)
	}
	sort.Strings(names)
	return names
}

// TestCommandSurfaceBudgetNotExceeded fails when a new top-level command lands
// while the core is supposed to be shrinking.
func TestCommandSurfaceBudgetNotExceeded(t *testing.T) {
	root := repoRoot(t)
	names := topLevelCommandNames(t, root)
	budget := readSurfaceBudget(t, root)

	if len(names) > budget {
		t.Fatalf("the core registers %d top-level commands but the budget is %d.\n"+
			"A new top-level command needs an ADR 0016 amendment, or belongs in a plugin.\nCommands: %s",
			len(names), budget, strings.Join(names, " "))
	}
}

// checkBudgetTight is the whole surface-budget rule once every canon move has
// landed (P7-CANON-17, ADR 0016): the budget equals the visible top-level count.
// Above reality the ratchet has slack that lets a new command in unnoticed;
// below reality the surface already regrew.
func checkBudgetTight(count, budget int) error {
	switch {
	case count > budget:
		return fmt.Errorf("the core registers %d top-level commands but the budget is %d", count, budget)
	case count < budget:
		return fmt.Errorf("the budget is %d but the core registers only %d top-level commands: lower %s to %d", budget, count, surfaceBudgetFile, count)
	}
	return nil
}

// TestCommandSurfaceBudgetIsTight requires the budget to equal reality. The
// fixture subtests pin both directions so the rule cannot go vacuous (a budget
// one higher fails, as does one lower); the real-data subtest applies it to the
// committed inventory.
func TestCommandSurfaceBudgetIsTight(t *testing.T) {
	t.Run("budget equal to reality passes", func(t *testing.T) {
		if err := checkBudgetTight(20, 20); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("budget one higher than reality fails", func(t *testing.T) {
		if err := checkBudgetTight(20, 21); err == nil {
			t.Fatal("a budget one above reality passed")
		}
	})
	t.Run("budget below reality fails", func(t *testing.T) {
		if err := checkBudgetTight(21, 20); err == nil {
			t.Fatal("a budget below reality passed")
		}
	})
	t.Run("committed budget", func(t *testing.T) {
		root := repoRoot(t)
		names := topLevelCommandNames(t, root)
		if len(names) == 0 {
			t.Fatal("inventory is empty")
		}
		if err := checkBudgetTight(len(names), readSurfaceBudget(t, root)); err != nil {
			t.Fatal(err)
		}
	})
}

// ceilingCmd is the part of a registry command the canon ceiling reads.
type ceilingCmd struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Parent string `json:"parent"`
	Hidden bool   `json:"hidden"`
	Canon  string `json:"canon"`
}

// canonCeilingProblems returns what breaks the permanent ceiling (review F11):
// the budget above the number of canon verbs, or a visible depth-1 command that
// is neither a canon verb nor a canon plugin. cobra's own `help` (canon builtin)
// is the one framework command that stays visible; it is not counted in the
// inventory and cannot grow the surface.
func canonCeilingProblems(budget int, verbs []string, cmds []ceilingCmd) []string {
	var p []string
	if budget > len(verbs) {
		p = append(p, fmt.Sprintf("budget %d exceeds the %d canon verbs (ADR 0016)", budget, len(verbs)))
	}
	isVerb := map[string]bool{}
	for _, v := range verbs {
		isVerb[v] = true
	}
	for _, c := range cmds {
		if c.Parent != "nself" || c.Hidden {
			continue
		}
		if isVerb[c.Name] || c.Canon == "plugin" || (c.Name == "help" && c.Canon == "builtin") {
			continue
		}
		p = append(p, fmt.Sprintf("%s is visible at depth 1 but is neither a canon verb nor a canon plugin (canon: %s)", c.Path, c.Canon))
	}
	return p
}

// TestCanonCeiling is the permanent ceiling beneath the equality ratchet: the
// budget file can never exceed the canon verb list and the v1.5 registry can
// show no visible top-level command outside the canon.
func TestCanonCeiling(t *testing.T) {
	verbs := []string{"init", "start", "add"}
	core := []ceilingCmd{
		{Path: "nself init", Name: "init", Parent: "nself", Canon: "core"},
		{Path: "nself admin", Name: "admin", Parent: "nself", Canon: "plugin"},
		{Path: "nself help", Name: "help", Parent: "nself", Canon: "builtin"},
		{Path: "nself dev", Name: "dev", Parent: "nself", Hidden: true, Canon: "deprecated-shim"},
		{Path: "nself config get", Name: "get", Parent: "nself config", Canon: "subcommand"},
	}
	t.Run("fixture: clean registry passes", func(t *testing.T) {
		if p := canonCeilingProblems(3, verbs, core); len(p) != 0 {
			t.Fatalf("problems: %v", p)
		}
	})
	t.Run("fixture: budget of 21 against 20 verbs fails", func(t *testing.T) {
		v := make([]string, 20)
		for i := range v {
			v[i] = fmt.Sprintf("verb%d", i)
		}
		if p := canonCeilingProblems(21, v, nil); len(p) != 1 {
			t.Fatalf("problems = %v, want the budget ceiling", p)
		}
	})
	t.Run("fixture: a visible non-verb top-level command fails", func(t *testing.T) {
		cmds := append(append([]ceilingCmd{}, core...), ceilingCmd{Path: "nself extra", Name: "extra", Parent: "nself", Canon: "core"})
		p := canonCeilingProblems(3, verbs, cmds)
		if len(p) != 1 || !strings.Contains(p[0], "nself extra") {
			t.Fatalf("problems = %v, want nself extra named", p)
		}
	})
	t.Run("committed budget and v1.5 registry", func(t *testing.T) {
		root := repoRoot(t)
		raw, err := canon.LoadRaw()
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(root, ".github", "command-registry.json"))
		if err != nil {
			t.Fatalf("read registry: %v (run `make cmd-inventory`)", err)
		}
		var doc struct {
			Commands []ceilingCmd `json:"commands"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("parse registry: %v", err)
		}
		if len(doc.Commands) == 0 || len(raw.Verbs) == 0 {
			t.Fatal("registry or canon verb list is empty")
		}
		if p := canonCeilingProblems(readSurfaceBudget(t, root), raw.Verbs, doc.Commands); len(p) != 0 {
			t.Fatalf("canon ceiling broken:\n%s", strings.Join(p, "\n"))
		}
	})
}

// goldenPathSpellings is the floor beneath the ratchet. Whatever else moves out
// to a plugin or under a hub, a fresh install must be able to create, build and
// run a stack, diagnose it, and extend itself with no plugin installed.
var goldenPathSpellings = []string{
	"init", "build", "start", "stop", "restart", "status", "logs", "urls",
	"doctor", "config", "env", "secrets", "db", "backup", "deploy",
	"plugin", "install", "version",
}

// regCommand is the part of a registry command the golden-path check reads.
type regCommand struct {
	Path     string  `json:"path"`
	Canon    string  `json:"canon"`
	Target   *string `json:"target"`
	Runnable bool    `json:"runnable"`
}

// resolveGoldenPath returns the spellings that do not resolve to a runnable
// core command, or to a deprecated-shim whose target is one. A shim target that
// is itself a shim does not count (the registry forbids shim chains).
func resolveGoldenPath(cmds []regCommand, spellings []string) []string {
	byPath := map[string]regCommand{}
	for _, c := range cmds {
		byPath[c.Path] = c
	}
	runnable := func(c regCommand, ok bool) bool {
		return ok && c.Runnable && c.Canon != "plugin" && c.Canon != "deprecated-shim"
	}
	var bad []string
	for _, sp := range spellings {
		c, ok := byPath["nself "+sp]
		switch {
		case runnable(c, ok):
		case ok && c.Canon == "deprecated-shim" && c.Target != nil && runnable(byPath[*c.Target], byPath[*c.Target].Path != ""):
		default:
			bad = append(bad, sp)
		}
	}
	return bad
}

// TestGoldenPathSpellingsResolve reads the committed registry (the v1.5
// contract) and checks every golden-path spelling resolves, so a move Ticket
// that relocates one of them leaves a working shim behind. The fixture
// subtests prove the check fails when a spelling resolves to nothing.
func TestGoldenPathSpellingsResolve(t *testing.T) {
	t.Run("fixture: urls resolves to nothing", func(t *testing.T) {
		cmds := []regCommand{{Path: "nself init", Canon: "core", Runnable: true}}
		bad := resolveGoldenPath(cmds, []string{"init", "urls"})
		if len(bad) != 1 || bad[0] != "urls" {
			t.Fatalf("bad = %v, want [urls]", bad)
		}
	})
	t.Run("fixture: shim to a runnable target resolves", func(t *testing.T) {
		tgt := "nself config urls"
		cmds := []regCommand{
			{Path: "nself urls", Canon: "deprecated-shim", Target: &tgt},
			{Path: "nself config urls", Canon: "subcommand", Runnable: true},
		}
		if bad := resolveGoldenPath(cmds, []string{"urls"}); len(bad) != 0 {
			t.Fatalf("bad = %v", bad)
		}
	})
	t.Run("fixture: shim to a missing or non-runnable target fails", func(t *testing.T) {
		tgt := "nself config urls"
		cmds := []regCommand{
			{Path: "nself urls", Canon: "deprecated-shim", Target: &tgt},
			{Path: "nself config urls", Canon: "subcommand", Runnable: false},
		}
		if bad := resolveGoldenPath(cmds, []string{"urls"}); len(bad) != 1 {
			t.Fatalf("bad = %v, want [urls]", bad)
		}
		if bad := resolveGoldenPath(cmds[:1], []string{"urls"}); len(bad) != 1 {
			t.Fatalf("missing target: bad = %v, want [urls]", bad)
		}
	})
	t.Run("committed registry", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "command-registry.json"))
		if err != nil {
			t.Fatalf("read registry: %v (run `make cmd-inventory`)", err)
		}
		var doc struct {
			Commands []regCommand `json:"commands"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("parse registry: %v", err)
		}
		if len(doc.Commands) == 0 {
			t.Fatal("registry has no commands")
		}
		if bad := resolveGoldenPath(doc.Commands, goldenPathSpellings); len(bad) > 0 {
			t.Fatalf("golden-path spelling(s) resolve to no runnable core command: %s",
				strings.Join(bad, " "))
		}
	})
}
