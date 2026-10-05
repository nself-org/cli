package commands

// Registry golden gate and cross-checks on the real command tree.
//
// Purpose: .github/command-registry.json documents contract
// cli.command-registry v1. These tests fail `go test ./...` when it drifts
// from the cobra tree + canon.yaml, when canon.yaml and the deprecation
// registry contradict each other, or when the counts disagree with the
// inventory the surface budget is checked against.
//
// Constraints: the committed file is the v1.5 contract, so the golden build
// selects v1.5 mode (compattest.Set). Nothing here runs a command body.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/deprecation"
)

const committedRegistry = "../../.github/command-registry.json"

// generatedLine is the first-key marker the generator writes; the golden
// compares the document without it.
var generatedLine = regexp.MustCompile(`(?m)\A\{\n  "_generated": "[^\n]*",\n`)

// goldenChild marks the child process that runs the golden check. Other tests
// in this package ResetFlags and redeclare flags on the real commands (for
// example env_target_test.go on `env target add`), so after them the live
// tree no longer matches what the shipped binary registers. The golden
// therefore runs the check in a fresh copy of this test binary, whose tree is
// exactly what init() registered, independent of test order.
const goldenChild = "NSELF_REGISTRY_GOLDEN_CHILD"

// goldenSentinel is printed by the child only after the byte comparison
// succeeded. The wrapper requires it, so a child that ran no test (renamed
// test, wrong -run pattern) cannot pass silently: `go test` exits 0 then.
const goldenSentinel = "REGISTRY-GOLDEN-CHECKED"

func TestRegistryGolden(t *testing.T) {
	if os.Getenv(goldenChild) == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRegistryGolden$", "-test.count=1", "-test.v")
		cmd.Env = append(os.Environ(), goldenChild+"=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("golden check in a fresh process failed: %v\n%s", err, out)
		}
		if !bytes.Contains(out, []byte(goldenSentinel)) || !bytes.Contains(out, []byte("--- PASS: TestRegistryGolden")) {
			t.Fatalf("the fresh process did not run the golden check (no %s and PASS line); output:\n%s", goldenSentinel, out)
		}
		return
	}
	compattest.Set(t, true)
	reattachRealTree()
	defer PrepareTreeForGeneration()() // the committed file documents the prepared v1.5 tree
	reg, err := BuildRegistry(true)
	if err != nil {
		t.Fatalf("build registry: %v", err)
	}
	got, err := reg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.ReadFile(committedRegistry)
	if err != nil {
		t.Fatalf("read %s: %v (run `make cmd-inventory`)", committedRegistry, err)
	}
	if !generatedLine.Match(file) {
		t.Fatalf("%s must start with the _generated marker (run `make cmd-inventory`)", committedRegistry)
	}
	want := generatedLine.ReplaceAll(file, []byte("{\n"))
	if !bytes.Equal(got, want) {
		t.Fatalf("the command registry changed but %s was not regenerated.\n"+
			"Run `make cmd-inventory` and commit the result. A new command also needs one "+
			"line in internal/canon/canon.yaml (see wiki Command-Registry). First difference:\n%s",
			committedRegistry, firstDiff(got, want))
	}
	t.Log(goldenSentinel)
}

// firstDiff names the first differing line, for a readable failure.
func firstDiff(got, want []byte) string {
	g, w := strings.Split(string(got), "\n"), strings.Split(string(want), "\n")
	for i := 0; i < len(g) && i < len(w); i++ {
		if g[i] != w[i] {
			return "line " + strconv.Itoa(i+1) + ": live " + g[i] + " | committed " + w[i]
		}
	}
	return "length differs: live " + strconv.Itoa(len(g)) + " lines, committed " + strconv.Itoa(len(w))
}

// Epic cross-check: the declared canon and internal/deprecation/registry.yaml
// must agree about which commands are shims, and every canon row must show up as
// a deprecated-shim in the v1.5 registry (P7-CANON-21).
func TestCanonShimsMatchDeprecationRegistry(t *testing.T) {
	reattachRealTree()
	defer PrepareTreeForGeneration()()
	reg, err := BuildRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	dep, err := deprecation.LoadEmbeddedRegistry()
	if err != nil {
		t.Fatalf("deprecation registry: %v", err)
	}
	aliases := map[string]bool{}
	byPath := map[string]cmdregistry.Command{}
	for _, c := range reg.Commands {
		byPath[c.Path] = c
		for _, a := range c.Aliases {
			aliases[c.Parent+" "+a] = true
		}
	}
	raw, err := canon.LoadRaw()
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]string{} // old path -> new path
	for _, list := range [][]canon.Row{raw.Moves, raw.Shims, raw.RetiredHubs} {
		for _, r := range list {
			rows["nself "+r.From] = "nself " + r.To
		}
	}
	// every moves/shims/retired_hubs row is a deprecated-shim in the v1.5 registry, targeting its `to`
	for from, to := range rows {
		c, ok := byPath[from]
		switch {
		case !ok:
			t.Errorf("canon row %q has no entry in the v1.5 registry", from)
		case c.Canon != canon.CanonShim:
			t.Errorf("canon row %q has canon %q in the v1.5 registry, want %s", from, c.Canon, canon.CanonShim)
		case c.Target == nil || *c.Target != to:
			t.Errorf("canon row %q: target %v, want %q", from, c.Target, to)
		}
	}
	checkDeprecationItems(t, dep, byPath, aliases)
	// every shim is announced: by a registry item or by cobra's Deprecated.
	for _, c := range reg.Commands {
		if c.Canon != canon.CanonShim {
			continue
		}
		_, inDep := dep.Lookup(c.Path)
		if !inDep && (c.Deprecated == nil || *c.Deprecated == "") {
			t.Errorf("shim %q has no deprecation-registry item and no cobra Deprecated text", c.Path)
		}
		if c.Target == nil {
			t.Errorf("shim %q has no target", c.Path)
		}
	}
	// a shim is allowed only with its green equivalence test (D5): the named Go test must exist here
	var equiv []string
	for _, r := range raw.Shims {
		equiv = append(equiv, r.Equivalence)
	}
	if miss := missingTests(t, equiv); len(miss) > 0 {
		t.Errorf("shim equivalence tests missing from cmd/commands: %v", miss)
	}
	if miss := missingTests(t, []string{"TestCanonShimsMatchDeprecationRegistry", "TestNoSuchEquivalenceXYZ"}); len(miss) != 1 || miss[0] != "TestNoSuchEquivalenceXYZ" {
		t.Errorf("the equivalence-test finder is broken: %v", miss)
	}
	t.Logf("%d canon rows checked against the v1.5 registry of the real tree", len(rows))

	// the same invariants on the fixture canon, where every row kind exists: the
	// prepared fixture tree must build a registry from the fixture's v1.5 view
	// (stubs and entries agree one to one) and carry one shim per row.
	fx, tb, root := fixtureCanon(t), fixtureTable(t), newFixtureTree()
	defer applyCanonWith(&tb, root, true)()
	view, err := fx.View(true)
	if err != nil {
		t.Fatal(err)
	}
	fixReg, err := cmdregistry.Build(root, view, map[string]any{}, cmdregistry.BuildOptions{V15: true})
	if err != nil {
		t.Fatalf("fixture registry: %v", err)
	}
	shims := 0
	for _, list := range [][]canon.Row{fx.Moves, fx.Shims, fx.RetiredHubs} {
		for _, r := range list {
			c, ok := fixReg.Lookup("nself " + r.From)
			if !ok || c.Canon != canon.CanonShim || c.Target == nil || *c.Target != "nself "+r.To {
				t.Errorf("fixture row %q: registry entry %+v", r.From, c)
			}
			if c != nil && (c.Deprecated == nil || *c.Deprecated == "") {
				t.Errorf("fixture shim %q has no cobra Deprecated text", r.From)
			}
			shims++
		}
	}
	if shims != len(fx.Moves)+len(fx.Shims)+len(fx.RetiredHubs) || shims == 0 {
		t.Errorf("checked %d fixture rows", shims)
	}
	// negative: a row whose stub is missing must be caught by the same registry build
	bare := newFixtureTree()
	if _, err := cmdregistry.Build(bare, view, map[string]any{}, cmdregistry.BuildOptions{V15: true}); err == nil {
		t.Error("an unprepared tree must not satisfy the v1.5 view")
	}
}

// checkDeprecationItems: a registry.yaml command item for a real command (not an
// alias) marks that command a canon deprecated-shim. An item naming no command is
// a removed or relocated one and keeps its warning entry; the replacement
// strings keep naming the v1.4 spelling, so they are not compared with rows.
func checkDeprecationItems(t *testing.T, dep *deprecation.Registry, byPath map[string]cmdregistry.Command, aliases map[string]bool) {
	t.Helper()
	for _, name := range dep.Names() {
		item, _ := dep.Lookup(name)
		if item.Type != deprecation.TypeCommand || aliases[name] {
			continue
		}
		if c, real := byPath[name]; real && c.Canon != canon.CanonShim {
			t.Errorf("%q has a deprecation-registry item but canon is %q (want %s)", name, c.Canon, canon.CanonShim)
		}
	}
}

func TestRegistryCountsMatchSurfaceBudget(t *testing.T) {
	reattachRealTree()
	reg, err := BuildRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	if n := inventoryLen(t); reg.Counts.TopLevel != n {
		t.Errorf("registry counts.top_level = %d, .github/command-inventory.json has %d", reg.Counts.TopLevel, n)
	}
	if reg.Counts.Commands != len(reg.Commands) {
		t.Errorf("counts.commands = %d, len(commands) = %d", reg.Counts.Commands, len(reg.Commands))
	}
}

// missingTests returns the names that are not a `func TestX(` in a *_test.go
// file of this package.
func missingTests(t *testing.T, names []string) []string {
	t.Helper()
	files, err := filepath.Glob("*_test.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no test files found: %v", err)
	}
	var all strings.Builder
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(b)
	}
	var miss []string
	for _, n := range names {
		if !strings.Contains(all.String(), "\nfunc "+n+"(") {
			miss = append(miss, n)
		}
	}
	return miss
}
