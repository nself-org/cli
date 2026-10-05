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
)

// surfaceBudgetFile holds the number of top-level commands the core binary is
// currently allowed to register.
//
// CLI-R11 extracts roughly thirty command families out to plugins, taking the
// core from 85 towards the approved target of ~35. That cannot land in one
// change, and while it is in progress the easiest way to lose ground is for a
// new top-level command to be added without anyone noticing. This is the same
// ratchet CLI-R12 uses for file sizes: the number may fall, never rise.
//
// Raising it requires a deliberate edit and an explanation in the commit
// message. The approved core list lives in the DECISIONS section of
// .claude/tasks/cli-review-tickets-2026-08-23.md.
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
			"CLI-R11 is shrinking this surface towards ~35 — a new top-level command\n"+
			"needs to justify itself, or belongs in a plugin.\nCommands: %s",
			len(names), budget, strings.Join(names, " "))
	}
}

// checkBudgetUpperBound is the whole surface-budget rule while the canon moves
// land: the budget may sit above reality (a move lowers the count before the
// budget file is edited) but never below it.
//
// P7-CANON-17 restores equality (budget == reality) once every move has landed.
func checkBudgetUpperBound(count, budget int) error {
	if count > budget {
		return fmt.Errorf("the core registers %d top-level commands but the budget is %d", count, budget)
	}
	return nil
}

// TestCommandSurfaceBudgetIsTight is upper-bound-only until P7-CANON-17: it
// passes when the budget is at or above reality and fails when it is below.
// The fixture subtests pin both directions so the rule cannot go vacuous; the
// real-data subtest applies it to the committed inventory.
func TestCommandSurfaceBudgetIsTight(t *testing.T) {
	// P7-CANON-17 restores equality
	t.Run("budget above reality passes", func(t *testing.T) {
		if err := checkBudgetUpperBound(20, 52); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("budget equal to reality passes", func(t *testing.T) {
		if err := checkBudgetUpperBound(52, 52); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("budget below reality fails", func(t *testing.T) {
		if err := checkBudgetUpperBound(53, 52); err == nil {
			t.Fatal("a budget below reality passed")
		}
	})
	t.Run("committed budget", func(t *testing.T) {
		root := repoRoot(t)
		names := topLevelCommandNames(t, root)
		if len(names) == 0 {
			t.Fatal("inventory is empty")
		}
		if err := checkBudgetUpperBound(len(names), readSurfaceBudget(t, root)); err != nil {
			t.Fatal(err)
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
