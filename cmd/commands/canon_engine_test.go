package commands

// Canon engine tests: tree application, undo, warnings, meta, E410, legacy
// chains, safety and the hub validators (P7-CANON-21).
//
// Purpose: prove every mechanic of EPIC D3/D10 on a fixture tree and a fixture
// table (testdata/canon/), in both compat modes, plus the generic per-move safety
// checks over the real tree with planted-fixture negatives.
//
// Inputs: testdata/canon/ (canon.yaml + domains/fixture.yaml), newFixtureTree().
//
// Constraints: tests select the mode with compattest, never a bare V15 branch
// outside the tests; the real RootCmd is mutated only through applyCanonWith and
// always restored by the returned undo.

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// --- fixture ---------------------------------------------------------------

func fxRun(*cobra.Command, []string) error      { return nil }
func fxHelp(c *cobra.Command, _ []string) error { return c.Help() }

func fxCmd(use string, run func(*cobra.Command, []string) error, kids ...*cobra.Command) *cobra.Command {
	c := &cobra.Command{Use: use, Short: "short " + use, RunE: run}
	c.AddCommand(kids...)
	return c
}

// fxExactStop is `service stop <name>`: it takes exactly one service name.
func fxExactStop() *cobra.Command {
	c := fxCmd("stop <name>", fxRun)
	c.Args = cobra.ExactArgs(1)
	return c
}

// newFixtureTree builds the v1.4 tree the fixture table (testdata/canon) moves.
func newFixtureTree() *cobra.Command {
	root := &cobra.Command{Use: "nself", Short: "fixture root", SilenceUsage: true, SilenceErrors: true}
	root.AddGroup(&cobra.Group{ID: groupConfig, Title: "Config:"})
	pf := root.PersistentFlags()
	pf.Bool("json", false, "")
	pf.Bool("no-deprecation-warnings", false, "")
	pf.Bool("no-monorepo", false, "")
	// --home is a fixture-only value flag: the real root has none, but the argv
	// tests need one flag that takes a separate value to prove the arity logic.
	pf.String("home", "", "")
	env := fxCmd("env", fxHelp, fxCmd("use", fxRun), fxCmd("target", fxHelp, fxCmd("add", fxRun)))
	env.Long = "Run nself env use <name>, then nself env target add. Other: nself service stop, nself envx."
	env.PersistentFlags().String("profile", "", "profile")
	env.GroupID = groupConfig
	secrets := fxCmd("secrets", fxHelp, fxCmd("get <key>", fxRun))
	secrets.Aliases = []string{"vault", "sec"}
	root.AddCommand(
		fxCmd("config", fxHelp, fxCmd("show", fxRun)), fxCmd("db", fxHelp, fxCmd("dump", fxRun), fxCmd("reset", fxRun), fxCmd("drop", fxRun)),
		fxCmd("deploy", fxHelp, fxCmd("promote", fxRun)), fxCmd("doctor", fxRun), env,
		fxCmd("migrate", fxHelp, fxCmd("up", fxRun), fxCmd("from-v099", fxRun)),
		fxCmd("ops", fxHelp, fxCmd("restart", fxRun), fxCmd("status", fxRun)),
		fxCmd("plugin", fxHelp, fxCmd("install", fxRun), fxCmd("marketplace", fxHelp, fxCmd("search", fxRun))),
		secrets, fxCmd("service", fxHelp, fxExactStop()), fxCmd("runner", fxHelp, fxCmd("ls", fxRun)),
		fxCmd("trust", fxRun, fxCmd("dns", fxRun), fxCmd("ssl", fxRun)), fxCmd("update", fxRun, fxCmd("check", fxRun)),
		fxCmd("restart", fxRun), fxCmd("status", fxRun), fxCmd("stop", fxRun), fxCmd("buy", fxRun),
		fxCmd("heal", fxRun), fxCmd("project-run", fxRun), fxCmd("completion", fxRun),
	)
	return root
}

// fixtureCanon loads testdata/canon through the real loader (so the fixture is
// itself valid canon in both modes).
func fixtureCanon(t testing.TB) *canon.File {
	t.Helper()
	f, err := canon.LoadFS(os.DirFS("testdata/canon"))
	if err != nil {
		t.Fatalf("fixture canon: %v", err)
	}
	return f
}

func fixtureTable(t testing.TB) canonTableT { return canon.BuildTable(fixtureCanon(t)) }

// snapshot serialises everything applyCanon may change, one line per command.
func snapshot(root *cobra.Command) string {
	var b strings.Builder
	var rec func(c *cobra.Command)
	rec = func(c *cobra.Command) {
		var ann []string
		for k, v := range c.Annotations {
			ann = append(ann, k+"="+v)
		}
		sort.Strings(ann)
		par := ""
		if c.Parent() != nil {
			par = c.Parent().CommandPath()
		}
		fmt.Fprintf(&b, "%s|par=%s|use=%s|al=%s|h=%v|dep=%s|g=%s|s=%s|l=%s|e=%s|args=%v|run=%v|dfp=%v|ann=%s\n",
			c.CommandPath(), par, c.Use, strings.Join(c.Aliases, ","), c.Hidden, c.Deprecated, c.GroupID,
			c.Short, c.Long, c.Example, c.Args != nil, c.Runnable(), c.DisableFlagParsing, strings.Join(ann, ","))
		for _, k := range c.Commands() {
			rec(k)
		}
	}
	rec(root)
	return b.String()
}

func at(root *cobra.Command, path string) *cobra.Command {
	return walkNames(root, strings.Fields(path))
}

func errCode(err error) string {
	var c *errs.CLIError
	if errors.As(err, &c) {
		return c.Code
	}
	return ""
}

// --- tree ------------------------------------------------------------------

func TestCanonEngineTree(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		root, tb := newFixtureTree(), fixtureTable(t)
		before := snapshot(root)
		undo := applyCanonWith(&tb, root, compat.V15())
		if !compat.V15() {
			if snapshot(root) != before {
				t.Fatal("v1.4: applyCanon changed the tree")
			}
			undo()
			return
		}
		if bad := canonUnresolved(&tb, newFixtureTree()); len(bad) > 0 {
			t.Fatalf("fixture rows do not resolve: %v", bad)
		}
		for _, p := range []string{"config env", "config env use", "config env target add", "config vault", "config vault get",
			"config trust", "config trust ssl", "config dns", "db up", "db import", "db import from-v099", "deploy ops",
			"deploy ops status", "store", "store buy", "doctor heal", "update project", "update project run", "restart", "stop"} {
			if c := at(root, p); c == nil || c.Annotations[annStub] != "" {
				t.Errorf("v1.5: %q is not a live command", p)
			}
		}
		for _, p := range []string{"runner", "runner ls", "plugin marketplace", "plugin marketplace search", "deploy ops restart"} {
			if at(root, p) != nil {
				t.Errorf("v1.5: %q must be gone from the tree", p)
			}
		}
		if c := at(root, "config env"); c.GroupID != "" || c.Annotations[annMovedFrom] != "env" {
			t.Errorf("relocated env: GroupID %q annotation %q", c.GroupID, c.Annotations[annMovedFrom])
		}
		if c := at(root, "config vault"); c.Use != "vault" || strings.Join(c.Aliases, ",") != "sec" || c.Name() != "vault" {
			t.Errorf("rename: Use %q aliases %v (alias equal to the new name must be dropped)", c.Use, c.Aliases)
		}
		if c := at(root, "config vault get"); c == nil || c.Use != "get <key>" {
			t.Errorf("a renamed subtree keeps its children: %+v", c)
		}
		for _, p := range []string{"store", "db import", "update project"} {
			if c := at(root, p); c.Runnable() || c.Annotations[annHub] != "1" {
				t.Errorf("created hub %q must be non-runnable and marked", p)
			}
		}
		if !at(root, "completion").Hidden {
			t.Error("builtin completion must be hidden")
		}
		for _, p := range []string{"env", "secrets", "ops", "migrate", "migrate up", "migrate from-v099", "trust", "trust dns", "buy", "heal", "project-run", "service stop", "ops restart"} {
			stub := at(root, p)
			if stub == nil || stub.Annotations[annStub] == "" {
				t.Errorf("v1.5: no stub at the old path %q", p)
				continue
			}
			if !stub.Hidden || !stub.DisableFlagParsing || !strings.Contains(stub.Deprecated, "'nself "+stub.Annotations[annStub]+"'") {
				t.Errorf("stub %q: hidden %v disableFlagParsing %v deprecated %q", p, stub.Hidden, stub.DisableFlagParsing, stub.Deprecated)
			}
			err := stub.RunE(stub, nil)
			if errCode(err) != "E401" || !strings.Contains(err.Error(), "nself "+stub.Annotations[annStub]) {
				t.Errorf("stub %q must fail with E401 naming the new spelling, got %v", p, err)
			}
		}
		long := at(root, "config env").Long
		want := "Run nself config env use <name>, then nself config env target add. Other: nself stop, nself envx."
		if long != want {
			t.Errorf("prose in a relocated subtree:\n got %q\nwant %q", long, want)
		}
		if at(root, "service").Long != "" || at(root, "config").Long != "" {
			t.Error("prose outside relocated subtrees must be untouched")
		}
		for _, p := range []string{"config", "db", "service", "config env target", "plugin"} {
			c := at(root, p)
			if p == "plugin" {
				if c.Args != nil {
					t.Error("plugin is not a help-only parent: it keeps no validator")
				}
				continue
			}
			if c.Args == nil || errCode(c.Args(c, []string{"bogus"})) != "E401" || c.Args(c, nil) != nil {
				t.Errorf("%q: want the unknown-subcommand validator", p)
			}
		}
		undo()
		if got := snapshot(root); got != before {
			t.Errorf("undo did not restore the tree:\n%s", diffLines(before, got))
		}
	})
}

func diffLines(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	var out []string
	seen := map[string]int{}
	for _, l := range al {
		seen[l]++
	}
	for _, l := range bl {
		if seen[l] > 0 {
			seen[l]--
		} else {
			out = append(out, "+ "+l)
		}
	}
	for l, n := range seen {
		for ; n > 0; n-- {
			out = append(out, "- "+l)
		}
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

func TestCanonEngineUndo(t *testing.T) {
	root, tb := newFixtureTree(), fixtureTable(t)
	before := snapshot(root)
	for i := 0; i < 3; i++ { // apply and undo repeatedly: undo must leave nothing behind
		undo := applyCanonWith(&tb, root, true)
		if snapshot(root) == before {
			t.Fatal("applyCanon changed nothing")
		}
		undo()
		if got := snapshot(root); got != before {
			t.Fatalf("round %d: undo did not restore the fixture tree exactly:\n%s", i, diffLines(before, got))
		}
	}
	// the real tree too: with the real (empty) table only the D-0062 validators change
	real := snapshot(RootCmd)
	undo := applyCanon(RootCmd, true)
	undo()
	if got := snapshot(RootCmd); got != real {
		t.Fatalf("real tree not restored:\n%s", diffLines(real, got))
	}
}

// --- warnings, meta, E410 --------------------------------------------------

func TestCanonEngineWarningOnce(t *testing.T) {
	root, tb := newFixtureTree(), fixtureTable(t)
	notes := func(args ...string) []canonNote {
		_, n, err := rewriteCanonArgsWith(&tb, root, args, true)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	const want = "warning: 'nself env' moved to 'nself config env'; the old spelling is removed in v1.6.0\n"
	resetCanonWarning()
	var out bytes.Buffer
	emitCanonWarning(&out, []string{"env", "use"}, notes("env", "use"), "", "")
	emitCanonWarning(&out, []string{"secrets", "get"}, notes("secrets", "get"), "", "") // a second rewrite in the same process
	if out.String() != want {
		t.Errorf("two rewrites must print one warning:\n got %q\nwant %q", out.String(), want)
	}
	for _, flag := range []string{"--no-deprecation-warnings", "--quiet"} { // --quiet is checked by argv text, as warnRelocatedCommand does
		resetCanonWarning()
		out.Reset()
		emitCanonWarning(&out, []string{"env", flag}, notes("env", flag), "", "")
		if out.Len() != 0 {
			t.Errorf("%s must silence the warning, got %q", flag, out.String())
		}
	}
	resetCanonWarning()
	out.Reset()
	emitCanonWarning(&out, []string{"env"}, nil, "", "")
	if out.Len() != 0 {
		t.Errorf("no rewrite, no warning; got %q", out.String())
	}
}

func TestCanonEngineMeta(t *testing.T) {
	output.ResetState()
	t.Cleanup(output.ResetState)
	root, tb := newFixtureTree(), fixtureTable(t)
	args := []string{"--json", "env", "use"}
	_, notes, err := rewriteCanonArgsWith(&tb, root, args, true)
	if err != nil {
		t.Fatal(err)
	}
	resetCanonWarning()
	var sink bytes.Buffer
	emitCanonWarning(&sink, args, notes, "", "")
	var doc bytes.Buffer
	if err := output.EmitData(output.Writer{Out: &doc, Err: io.Discard}, "config env use", map[string]string{"ok": "1"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"deprecations"`, `"old": "env"`, `"new": "config env"`, `"removal_at": "v1.6.0"`} {
		if !strings.Contains(doc.String(), want) {
			t.Errorf("JSON mode: meta lacks %s:\n%s", want, doc.String())
		}
	}
	output.ResetState() // text mode records nothing
	resetCanonWarning()
	emitCanonWarning(&sink, []string{"env", "use"}, notes, "", "")
	doc.Reset()
	_ = output.EmitData(output.Writer{Out: &doc, Err: io.Discard}, "x", 1)
	if strings.Contains(doc.String(), "deprecations") {
		t.Errorf("text mode must not record meta deprecations:\n%s", doc.String())
	}
}

func TestCanonEngineRemovedE410(t *testing.T) {
	root, tb := newFixtureTree(), fixtureTable(t)
	argv := []string{"plugin", "marketplace", "search", "x"}
	compattest.Both(t, func(t *testing.T) {
		got, notes, err := rewriteCanonArgsWith(&tb, root, argv, compat.V15())
		if !compat.V15() {
			if err != nil || len(notes) != 0 || !reflect.DeepEqual(got, argv) {
				t.Fatalf("v1.4: a removed row must not touch argv: %v %v %v", got, notes, err)
			}
			return
		}
		if errCode(err) != "E410" || !strings.Contains(err.Error(), "nself has no plugin marketplace") || !strings.Contains(err.Error(), "'nself plugin marketplace' was removed in v1.5.0") {
			t.Fatalf("want E410 with the row message, got %v", err)
		}
		if errs.ExitCodeFor(err) != 1 {
			t.Errorf("E410 exits %d, want 1", errs.ExitCodeFor(err))
		}
		if _, _, err := rewriteCanonArgsWith(&tb, root, []string{"plugin", "install", "x"}, true); err != nil {
			t.Errorf("a sibling of a removed command must still run: %v", err)
		}
		if e, ok := errs.Registry["E410"]; !ok || e.Exit != 1 || e.Category != "cli" {
			t.Errorf("E410 registry entry: %+v", e)
		}
	})
}

func TestCanonBareRetiredHubRewrite(t *testing.T) {
	root, tb := newFixtureTree(), fixtureTable(t)
	var moves []canonRowT
	for _, move := range tb.Moves {
		if strings.Join(move.From, " ") != "ops" {
			moves = append(moves, move)
		}
	}
	tb.Moves = moves
	tb.Moves = append(tb.Moves, canonRowT{From: sp("ops status"), To: sp("deploy ops")})
	tb.RetiredHubs = append(tb.RetiredHubs, canonRowT{From: sp("ops"), To: sp("deploy ops")})
	for _, tc := range []struct{ name, in, want, kind string }{
		{"retired help only", "migrate", "db", "retired"},
		{"retired help only flags", "migrate --json", "db --json", "retired"},
		{"retired help flag", "migrate --help", "db --help", "retired"},
		{"retired runnable", "ops", "deploy ops --help", "retired"},
		{"retired unknown child", "migrate bogus", "migrate bogus", ""},
		{"move", "env use", "config env use", "move"},
		{"shim", "service stop redis", "stop redis", "shim"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, notes, err := rewriteCanonArgsWith(&tb, root, strings.Fields(tc.in), true)
			if err != nil || strings.Join(got, " ") != tc.want {
				t.Fatalf("rewrite %q = %q, %v; want %q", tc.in, got, err, tc.want)
			}
			if tc.kind == "" && len(notes) != 0 || tc.kind != "" && (len(notes) != 1 || notes[0].Kind != tc.kind) {
				t.Fatalf("rewrite %q notes = %+v; want %q", tc.in, notes, tc.kind)
			}
		})
	}
}

// --- legacy chains ---------------------------------------------------------

func runChain(t *testing.T, tb canonTableT, root *cobra.Command, v15 bool, argv ...string) ([]string, string) {
	t.Helper()
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })
	os.Args = append([]string{"nself"}, argv...)
	resetCanonWarning()
	legacy := rewriteLegacyInvocation()
	args, notes, err := rewriteCanonArgsWith(&tb, root, processArgs(), v15)
	if err != nil {
		t.Fatal(err)
	}
	os.Args = append([]string{"nself"}, args...)
	return args, captureStderr(t, func() { warnLegacyChain(legacy, notes) })
}

func TestCanonEngineLegacyChain(t *testing.T) {
	tb := fixtureTable(t)
	cases := []struct {
		argv, want []string
		warn       string
	}{
		{[]string{"dns-setup"}, []string{"config", "dns"}, "warning: 'nself dns-setup' moved to 'nself config dns'; the old spelling is removed in v1.6.0\n"},
		{[]string{"ssl"}, []string{"config", "trust", "ssl"}, "warning: 'nself ssl' moved to 'nself config trust ssl'; the old spelling is removed in v1.6.0\n"},
		{[]string{"migrate-from-v099", "x"}, []string{"db", "import", "from-v099", "x"}, "warning: 'nself migrate-from-v099' moved to 'nself db import from-v099'; the old spelling is removed in v1.6.0\n"},
	}
	for _, c := range cases {
		root := newFixtureTree()
		got, stderr := runChain(t, tb, root, true, c.argv...)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("v1.5 %v: argv %v, want %v", c.argv, got, c.want)
		}
		if stderr != c.warn || strings.Count(stderr, "warning:") != 1 || strings.Contains(stderr, "DEPRECATED") {
			t.Errorf("v1.5 %v: want exactly one warning naming the final path:\n got %q\nwant %q", c.argv, stderr, c.warn)
		}
	}
	// a legacy spelling whose target did not move keeps the registry warning
	direct := captureStderr(t, func() { warnLegacySpelling("upgrade") })
	if _, got := runChain(t, tb, newFixtureTree(), true, "upgrade"); got != direct {
		t.Errorf("v1.5 legacy spelling without a canon row: want the registry warning %q, got %q", direct, got)
	}
	// v1.4: unchanged, even though the fixture table has rows for trust
	for _, c := range cases {
		got, stderr := runChain(t, tb, newFixtureTree(), false, c.argv...)
		canonical := legacySpellings[c.argv[0]].canonical
		if !reflect.DeepEqual(got, append(append([]string{}, canonical...), c.argv[1:]...)) {
			t.Errorf("v1.4 %v: argv %v", c.argv, got)
		}
		if want := captureStderr(t, func() { warnLegacySpelling(c.argv[0]) }); stderr != want || strings.Contains(stderr, "moved to") && !strings.Contains(want, "moved to") {
			t.Errorf("v1.4 %v: the legacy warning must be unchanged:\n got %q\nwant %q", c.argv, stderr, want)
		}
	}
}

// --- no YAML on the startup path -------------------------------------------

func TestCanonEngineNoCanonLoad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
	calls := 0
	saved := canonLoad
	canonLoad = func(b bool) (*canon.File, error) { calls++; return saved(b) }
	t.Cleanup(func() { canonLoad = saved })
	tb, root := fixtureTable(t), newFixtureTree()
	for _, v15 := range []bool{false, true} {
		_, _, _ = rewriteCanonArgsWith(&tb, root, []string{"env", "use"}, v15)
		_, _, _ = rewriteCanonArgs([]string{"env", "use"}, v15)
		applyCanonWith(&tb, root, v15)()
		applyCanon(RootCmd, v15)()
		_ = prepareTree(newFixtureTree(), v15, true)
	}
	if calls != 0 {
		t.Fatalf("the engine called canon.Load %d times", calls)
	}
	// static proof: the engine files name no canon loader at all
	for _, f := range []string{"canon_engine.go", "canon_engine_argv.go", "canon_engine_tree.go", "tree_prepare.go"} {
		file, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "canon" {
				switch sel.Sel.Name {
				case "Load", "LoadRaw", "LoadFS", "Effective", "Parse":
					t.Errorf("%s uses canon.%s: the startup path must read only the generated table", f, sel.Sel.Name)
				}
			}
			return true
		})
	}
}

// --- hubs (D-0062) ---------------------------------------------------------

func TestHubUnknownSubcommand(t *testing.T) {
	reattachRealTree()
	mountBuiltinFamiliesFromList(RootCmd)
	retired := map[string]string{}
	for _, row := range canonTable.RetiredHubs {
		retired[strings.Join(row.From, " ")] = strings.Join(row.To, " ")
	}
	// 1. every helpOnlyParents entry exists and is help-only: its body only prints help.
	for _, p := range helpOnlyParents {
		c := at(RootCmd, p)
		if c == nil || !c.HasSubCommands() || c.Args != nil || c.RunE == nil {
			t.Fatalf("helpOnlyParents entry %q is not a parent with a body and no Args", p)
		}
		var buf bytes.Buffer
		c.SetOut(&buf)
		if err := c.RunE(c, []string{"ignored"}); err != nil || !strings.Contains(buf.String(), "Usage:") {
			t.Errorf("%q is not help-only (err %v, output %q)", p, err, buf.String())
		}
		c.SetOut(nil)
	}
	// 2. v1.5 gives each of them E401 on an unknown subcommand and leaves the other parents alone.
	listed := map[string]bool{}
	expected := len(helpOnlyParents)
	for _, p := range helpOnlyParents {
		if to, ok := retired[p]; ok {
			// A retired hub can target a runnable moved leaf. Its bare argv
			// gets help from the rewrite, while the leaf keeps its own Args.
			runnableTarget := false
			for _, move := range canonTable.Moves {
				if strings.Join(move.To, " ") == to {
					runnableTarget = true
				}
			}
			if runnableTarget {
				expected--
			} else {
				listed[to] = true
			}
			continue // the old path is now an error stub, not a help-only parent
		}
		listed[p] = true
		// A move relocates the parent and the validator follows the command,
		// so the walk must expect it at its canonical path; the engine's own
		// old->new rewrite names it. Rewritten here, while the tree still
		// holds the old spelling the rewrite resolves against.
		if rw, _, err := rewriteCanonArgsWith(&canonTable, RootCmd, strings.Fields(p), true); err == nil {
			listed[strings.Join(rw, " ")] = true
		}
	}
	undo := applyCanon(RootCmd, true)
	for from, to := range retired {
		stub := at(RootCmd, from)
		if stub == nil || stub.Annotations[annStub] != to || errCode(stub.RunE(stub, []string{"bogus"})) != "E401" {
			t.Errorf("retired stub %q must name %q with E401", from, to)
		}
	}
	validated, other := 0, 0
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c != RootCmd && c.HasSubCommands() {
			path := strings.TrimPrefix(c.CommandPath(), "nself ")
			switch {
			case listed[path]:
				validated++
				if c.Args == nil || errCode(c.Args(c, []string{"bogus"})) != "E401" || c.Args(c, nil) != nil {
					t.Errorf("%q: want the unknown-subcommand validator", path)
				}
			case c.Args == nil:
				other++ // a parent that takes arguments of its own keeps cobra's behaviour
			}
		}
		for _, ch := range c.Commands() {
			walk(ch)
		}
	}
	walk(RootCmd)
	undo()
	if validated != expected {
		t.Errorf("validated %d of %d help-only parents", validated, expected)
	}
	t.Logf("%d help-only parents validated; %d parents with their own arguments left alone", validated, other)

	// 3. a hub with no body at all: cobra prints help before any validator, so the preflight enforces it.
	root, tb := newFixtureTree(), fixtureTable(t)
	defer applyCanonWith(&tb, root, true)()
	n := 0
	var hubs func(c *cobra.Command)
	hubs = func(c *cobra.Command) {
		if c != root && c.HasSubCommands() && !c.Runnable() {
			n++
			path := strings.Fields(strings.TrimPrefix(c.CommandPath(), "nself "))
			if err := hubPreflight(root, append(append([]string{}, path...), "bogus")); errCode(err) != "E401" {
				t.Errorf("hub %v: want E401 for an unknown subcommand, got %v", path, err)
			}
			if err := hubPreflight(root, path); err != nil {
				t.Errorf("hub %v: no extra words, no error; got %v", path, err)
			}
			if err := hubPreflight(root, append(append([]string{}, path...), "--json", "bogus")); err != nil {
				t.Errorf("hub %v: words after a flag are not located, got %v", path, err)
			}
		}
		for _, ch := range c.Commands() {
			hubs(ch)
		}
	}
	hubs(root)
	if n != 3 {
		t.Errorf("fixture has %d body-less hubs with subcommands, want 3 (db import, store, update project)", n)
	}
	// a runnable command below a hub is not an unknown subcommand
	if err := hubPreflight(root, []string{"store", "buy", "x"}); err != nil {
		t.Errorf("store buy x: %v", err)
	}
	// end to end on cobra: a help-only parent exits non-zero in v1.5, prints help in v1.4
	for _, v15 := range []bool{false, true} {
		r2 := newFixtureTree()
		if v15 {
			defer applyCanonWith(&tb, r2, true)()
		}
		var out bytes.Buffer
		r2.SetOut(&out)
		r2.SetErr(&out)
		r2.SetArgs([]string{"service", "bogus"})
		err := r2.Execute()
		if v15 && errCode(err) != "E401" || !v15 && err != nil {
			t.Errorf("service bogus (v1.5=%v): err %v", v15, err)
		}
	}
}

// --- safety (generic, per move) ---------------------------------------------

type safetyFacts struct {
	flags      map[string]string
	pre, args  uintptr
	srcSafe    bool
	repoScoped bool
	engineArgs bool
	leaf       string
}

func factsOf(c *cobra.Command) safetyFacts {
	f := safetyFacts{flags: map[string]string{}, leaf: c.Name(), srcSafe: isSourceSafeCommand(c.Name()), repoScoped: isRepoScopedCommand(c.Name())}
	// The flags the command answers to: its own, plus the persistent flags of
	// every ancestor. cobra's LocalFlags/InheritedFlags merge and cache, so a
	// moved command would keep reporting the old parent's flags; read the
	// declarations instead.
	c.Flags().VisitAll(func(x *pflag.Flag) { f.flags[x.Name] = x.Value.Type() })
	for p := c; p != nil; p = p.Parent() {
		p.PersistentFlags().VisitAll(func(x *pflag.Flag) { f.flags[x.Name] = x.Value.Type() })
	}
	for p := c; p != nil; p = p.Parent() { // cobra runs the nearest persistent pre-run hook
		if p.PersistentPreRunE != nil {
			f.pre = reflect.ValueOf(p.PersistentPreRunE).Pointer()
			break
		}
		if p.PersistentPreRun != nil {
			f.pre = reflect.ValueOf(p.PersistentPreRun).Pointer()
			break
		}
	}
	if c.Args != nil {
		f.args = reflect.ValueOf(c.Args).Pointer()
		f.engineArgs = f.args == reflect.ValueOf(hubUnknownSubcommand).Pointer()
	}
	return f
}

// canonSafetyProblems applies t to root and reports every moved command that
// lost a flag, whose nearest PersistentPreRunE changed, whose leaf-name source
// guard decision changed, or whose Args validator changed (the engine's own
// unknown-subcommand validator on a help-only parent is the one allowed change),
// plus every row the engine could not apply. mutate, when set, runs after the
// application (planted-fixture tests).
func canonSafetyProblems(t *canonTableT, root *cobra.Command, mutate func(moved map[string]*cobra.Command)) []string {
	p := canonUnresolved(t, root)
	moved, before := map[string]*cobra.Command{}, map[string]safetyFacts{}
	for _, r := range t.Moves {
		if n := walkNames(root, r.From); n != nil {
			k := strings.Join(r.From, " ")
			moved[k], before[k] = n, factsOf(n)
		}
	}
	defer applyCanonWith(t, root, true)()
	if mutate != nil {
		mutate(moved)
	}
	for k, b := range before {
		a := factsOf(moved[k])
		if moved[k].Annotations[annMovedFrom] != k {
			p = append(p, fmt.Sprintf("%q: the engine did not apply the move (name taken or destination missing)", k))
		}
		for name, typ := range b.flags {
			if at, ok := a.flags[name]; !ok {
				p = append(p, fmt.Sprintf("%q loses flag --%s", k, name))
			} else if at != typ {
				p = append(p, fmt.Sprintf("%q flag --%s changes type %s -> %s", k, name, typ, at))
			}
		}
		if a.pre != b.pre {
			p = append(p, fmt.Sprintf("%q: its nearest PersistentPreRunE changes", k))
		}
		if a.srcSafe != b.srcSafe || a.repoScoped != b.repoScoped {
			p = append(p, fmt.Sprintf("%q: isSourceSafeCommand/isRepoScopedCommand decision changes with the leaf name %q -> %q", k, b.leaf, a.leaf))
		}
		if a.args != b.args && !(b.args == 0 && a.engineArgs) {
			p = append(p, fmt.Sprintf("%q: its Args validator changes", k))
		}
	}
	sort.Strings(p)
	return p
}

func TestCanonEngineSafety(t *testing.T) {
	reattachRealTree()
	// the generated table over the real tree: every row must be safe (none exist until the move Tickets land)
	moves := len(canonTable.Moves)
	if p := canonSafetyProblems(&canonTable, RootCmd, nil); len(p) > 0 {
		t.Errorf("the generated table is unsafe:\n  %s", strings.Join(p, "\n  "))
	}
	t.Logf("generated table: %d moves, %d shims, %d retired hubs checked on the real tree", moves, len(canonTable.Shims), len(canonTable.RetiredHubs))

	// the same checks on moves of real commands: sane moves pass, so a pass means something
	sane := canonTableT{Moves: []canonRowT{
		{From: []string{"secrets"}, To: []string{"config", "secrets"}},
		{From: []string{"oauth"}, To: []string{"config", "oauth"}},
		{From: []string{"telemetry"}, To: []string{"config", "telemetry"}},
	}}
	if p := canonSafetyProblems(&sane, RootCmd, nil); len(p) > 0 {
		t.Errorf("sane moves of real commands must pass:\n  %s", strings.Join(p, "\n  "))
	}

	// planted fixtures: each rule must fail on its own plant
	plant := func(name, want string, tb canonTableT, build func() *cobra.Command, mutate func(map[string]*cobra.Command)) {
		t.Helper()
		p := canonSafetyProblems(&tb, build(), mutate)
		if len(p) == 0 || !strings.Contains(strings.Join(p, "\n"), want) {
			t.Errorf("planted %s: want a problem containing %q, got %v", name, want, p)
		}
	}
	row := func(from, to string) canonTableT {
		return canonTableT{Moves: []canonRowT{{From: strings.Fields(from), To: strings.Fields(to)}}}
	}
	plant("lost flag", "loses flag --format", row("config show", "status show"), func() *cobra.Command {
		r := newFixtureTree()
		at(r, "config").PersistentFlags().String("format", "", "")
		at(r, "status").AddCommand(fxCmd("keep", fxRun))
		return r
	}, nil)
	plant("changed PersistentPreRunE", "PersistentPreRunE changes", row("buy", "store buy"), func() *cobra.Command {
		r := newFixtureTree()
		r.PersistentPreRunE = func(*cobra.Command, []string) error { return nil }
		store := fxCmd("store", fxHelp)
		store.PersistentPreRunE = func(*cobra.Command, []string) error { return errors.New("different guard") }
		r.AddCommand(store)
		return r
	}, nil)
	plant("source-guard decision", "isSourceSafeCommand", row("doctor", "status diagnose"), newFixtureTree, nil)
	plant("repo-scoped decision", "isRepoScopedCommand", row("trust", "config ci"), func() *cobra.Command {
		r := newFixtureTree()
		return r
	}, nil)
	plant("changed Args", "Args validator changes", row("buy", "store buy"), newFixtureTree, func(m map[string]*cobra.Command) { m["buy"].Args = cobra.NoArgs })
	plant("unresolved destination", "destination parent", row("buy", "nowhere buy"), newFixtureTree, nil)
}

// --- prepareInvocation end to end --------------------------------------------

// TestCanonEnginePrepare drives the whole pre-dispatch path on the fixture tree:
// argv rewrite, tree relocation, destination check, warning, hub preflight.
func TestCanonEnginePrepare(t *testing.T) {
	tb := fixtureTable(t)
	run := func(root *cobra.Command, v15, mount bool, argv ...string) ([]string, string, error) {
		t.Helper()
		compattest.Set(t, v15)
		savedArgs, savedMount := os.Args, mountInstalledPlugins
		t.Cleanup(func() { os.Args, mountInstalledPlugins = savedArgs, savedMount })
		if mount {
			mountInstalledPlugins = func(r *cobra.Command) {
				ci := fxCmd("ci", nil, fxCmd("nodes", fxRun))
				ci.Annotations = map[string]string{"nself.plugin": "ci"}
				r.AddCommand(ci)
			}
		}
		os.Args = append([]string{"nself"}, argv...)
		resetCanonWarning()
		var err error
		stderr := captureStderr(t, func() { err = prepareInvocationWith(&tb, root) })
		return os.Args[1:], stderr, err
	}
	const warn = "warning: 'nself env' moved to 'nself config env'; the old spelling is removed in v1.6.0\n"
	cases := []struct {
		name         string
		v15, mount   bool
		argv, want   string
		stderr, code string
		tweak        func(*cobra.Command)
	}{
		{name: "v1.5 old spelling", v15: true, argv: "env use x", want: "config env use x", stderr: warn},
		{name: "v1.5 silenced", v15: true, argv: "--no-deprecation-warnings env use x", want: "--no-deprecation-warnings config env use x"},
		{name: "v1.5 canonical", v15: true, argv: "config env use x", want: "config env use x"},
		{name: "v1.5 breakout without the plugin mounted keeps argv", v15: true, argv: "runner ls", want: "runner ls"},
		{name: "v1.5 breakout with the plugin mounted", v15: true, mount: true, argv: "runner ls", want: "ci nodes ls",
			stderr: "warning: 'nself runner' moved to 'nself ci nodes'; the old spelling is removed in v1.6.0\n"},
		{name: "v1.5 removed", v15: true, argv: "plugin marketplace search x", code: "E410"},
		{name: "v1.5 hub without a body", v15: true, argv: "store bogus", want: "store bogus", code: "E401"},
		{name: "v1.5 a taken destination name keeps the old spelling", v15: true, argv: "env use x", want: "env use x",
			tweak: func(r *cobra.Command) { at(r, "config").AddCommand(fxCmd("env", fxRun)) }},
		{name: "v1.4 new spelling under an existing hub keeps its meaning", argv: "config env use x", want: "config env use x"},
		{name: "v1.4 new spelling under a created hub", argv: "store buy x", want: "buy x"},
		{name: "v1.4 old spelling", argv: "env use x", want: "env use x"},
		{name: "v1.4 hub words are not checked", argv: "store bogus", want: "store bogus"},
		{name: "v1.4 removed rows do nothing", argv: "plugin marketplace search x", want: "plugin marketplace search x"},
	}
	for _, c := range cases {
		root := newFixtureTree()
		if c.tweak != nil {
			c.tweak(root)
		}
		got, stderr, err := run(root, c.v15, c.mount, sp(c.argv)...)
		if errCode(err) != c.code {
			t.Errorf("%s: error code %q (%v), want %q", c.name, errCode(err), err, c.code)
		}
		if c.want != "" && strings.Join(got, " ") != c.want {
			t.Errorf("%s: argv %q, want %q", c.name, strings.Join(got, " "), c.want)
		}
		if stderr != c.stderr {
			t.Errorf("%s: stderr %q, want %q", c.name, stderr, c.stderr)
		}
	}
}

// takesArgsParents are the real parents with subcommands, a body and no Args
// validator that read arguments or do real work: they are NOT help-only. Every
// other such parent must be in helpOnlyParents (TestHelpOnlyParentsComplete).
var takesArgsParents = []string{"account", "admin", "bundle", "ci", "ci eval", "db drift", "doctor", "health", "migrate", "plugin", "trust", "update"}

// unclassifiedParents returns every parent under root that has subcommands, a
// body and no Args validator but is in neither list, plus every list entry that
// names no such parent, so each new parent forces a decision and a stale entry
// cannot linger.
func unclassifiedParents(root *cobra.Command, helpOnly, takesArgs []string) []string {
	listed := map[string]string{}
	for _, p := range helpOnly {
		listed[p] = "helpOnlyParents"
	}
	for _, p := range takesArgs {
		if prev, dup := listed[p]; dup {
			return []string{p + " is in both " + prev + " and takesArgsParents"}
		}
		listed[p] = "takesArgsParents"
	}
	var problems []string
	seen := map[string]bool{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		path := strings.TrimPrefix(c.CommandPath(), root.CommandPath()+" ")
		if c != root && c.HasSubCommands() && c.Args == nil && c.Runnable() {
			seen[path] = true
			if listed[path] == "" {
				problems = append(problems, "parent "+path+" has a body and no Args but is classified in neither list")
			}
		}
		for _, ch := range c.Commands() {
			walk(ch)
		}
	}
	walk(root)
	for p, l := range listed {
		if !seen[p] {
			problems = append(problems, l+" entry "+p+" is not a parent with a body and no Args")
		}
	}
	sort.Strings(problems)
	return problems
}

func TestHelpOnlyParentsComplete(t *testing.T) {
	reattachRealTree()
	if p := unclassifiedParents(RootCmd, helpOnlyParents, takesArgsParents); len(p) > 0 {
		t.Fatalf("classify every parent as help-only (helpOnlyParents) or arg-taking (takesArgsParents):\n  %s", strings.Join(p, "\n  "))
	}
	// planted: a new help-only parent, a stale entry and a double entry are all caught
	root := newFixtureTree()
	zz := fxCmd("zzhelponly", fxHelp, fxCmd("child", fxRun))
	root.AddCommand(zz)
	got := strings.Join(unclassifiedParents(root, []string{"config", "db", "env target", "ops", "plugin marketplace", "runner", "secrets", "service", "nosuch"}, []string{"trust", "update", "migrate", "plugin"}), "\n")
	for _, want := range []string{"parent zzhelponly has a body", "helpOnlyParents entry nosuch"} {
		if !strings.Contains(got, want) {
			t.Errorf("planted drift not caught (%q):\n%s", want, got)
		}
	}
	if p := unclassifiedParents(root, []string{"config"}, []string{"config"}); len(p) != 1 || !strings.Contains(p[0], "both") {
		t.Errorf("double entry: %v", p)
	}
}
