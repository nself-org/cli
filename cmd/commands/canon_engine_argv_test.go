package commands

// Canon engine argv tests (P7-CANON-21).
//
// Purpose: prove the argv rewrite maps old to new (v1.5) and new to old (v1.4)
// exactly where it should, honours root flags, `--flag=value` and `--`, and
// never turns an unknown or ambiguous argv into another command.
//
// Inputs: the fixture table and tree of canon_engine_test.go.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
)

func sp(s string) []string { return strings.Fields(s) }

func TestCanonEngineArgv(t *testing.T) {
	tb := fixtureTable(t)
	type tc struct{ in, v14, v15 string }
	cases := []tc{
		// renames, subtrees and flags in every position
		{in: "env use prod", v15: "config env use prod", v14: "env use prod"},
		{in: "--json env use", v15: "--json config env use", v14: "--json env use"},
		{in: "-q env", v15: "-q config env", v14: "-q env"},
		{in: "--home /tmp/x env use", v15: "--home /tmp/x config env use", v14: "--home /tmp/x env use"},
		{in: "--home=/tmp/x env use", v15: "--home=/tmp/x config env use", v14: "--home=/tmp/x env use"},
		{in: "env --profile p use", v15: "config env --profile p use", v14: "env --profile p use"},
		{in: "env use -- x env", v15: "config env use -- x env", v14: "env use -- x env"},
		{in: "secrets get k", v15: "config vault get k", v14: "secrets get k"},
		{in: "config vault get k", v15: "config vault get k", v14: "secrets get k"},
		{in: "config env use", v15: "config env use", v14: "env use"},
		{in: "store buy x", v15: "store buy x", v14: "buy x"},
		{in: "buy x", v15: "store buy x", v14: "buy x"},
		{in: "db import from-v099 f", v15: "db import from-v099 f", v14: "migrate from-v099 f"},
		{in: "db up", v15: "db up", v14: "migrate up"},
		// the longest prefix wins: a shim inside a moved subtree, a child move of a moved parent
		{in: "ops restart svc", v15: "restart svc", v14: "ops restart svc"},
		{in: "ops status", v15: "deploy ops status", v14: "ops status"},
		{in: "trust dns", v15: "config dns", v14: "trust dns"},
		{in: "config dns", v15: "config dns", v14: "trust dns"},
		{in: "trust ssl", v15: "config trust ssl", v14: "trust ssl"},
		{in: "service stop svc", v15: "stop svc", v14: "service stop svc"},
		// retired hubs forward whole; break-outs go to the plugin path (in v1.4 only moves are mapped)
		{in: "migrate from-v099 x", v15: "db import from-v099 x", v14: "migrate from-v099 x"},
		{in: "migrate bogus", v15: "db bogus", v14: "migrate bogus"},
		{in: "runner ls", v15: "ci nodes ls", v14: "runner ls"},
		{in: "ci nodes ls", v15: "ci nodes ls", v14: "ci nodes ls"},
		// canonical and unrelated spellings are never touched
		{in: "status", v15: "status", v14: "status"},
		{in: "restart svc", v15: "restart svc", v14: "restart svc"},
		{in: "config show", v15: "config show", v14: "config show"},
		{in: "plugin install x", v15: "plugin install x", v14: "plugin install x"},
	}
	// negatives: argv the engine must leave alone because it cannot be sure
	for _, s := range []string{
		"", "--", "-- env use", "--bogus env use", "--home", "-x env use", "envx use", "env2", "help env", "completion env",
		"exec env ls", "logs env", "status env", "--json", "-q -- env", "--home x", "-qj env",
		// a service named like a moved command is reached after `--` (D3: nself status -- <name>)
		"status -- env", "status -- trust", "status -- buy now",
	} {
		cases = append(cases, tc{in: s, v14: s, v15: s})
	}
	root := newFixtureTree()
	compattest.Both(t, func(t *testing.T) {
		for _, c := range cases {
			want := c.v14
			if compat.V15() {
				want = c.v15
			}
			got, _, err := rewriteCanonArgsWith(&tb, root, sp(c.in), compat.V15())
			if err != nil {
				t.Errorf("%q: %v", c.in, err)
				continue
			}
			if !reflect.DeepEqual(append([]string{}, got...), append([]string{}, sp(want)...)) {
				t.Errorf("%q: got %q, want %q", c.in, got, want)
			}
		}
	})
}

// A rewrite never mutates its input, never touches words after `--`, and never
// grows into a different destructive command: every rewritten argv keeps the
// tail verbatim and only ever replaces whole leading words named by one row.
func TestCanonEngineArgvNegative(t *testing.T) {
	tb := fixtureTable(t)
	root := newFixtureTree()
	in := sp("--home x env use prod -- env")
	keep := append([]string{}, in...)
	got, notes, err := rewriteCanonArgsWith(&tb, root, in, true)
	if err != nil || !reflect.DeepEqual(in, keep) {
		t.Fatalf("input mutated or error: %v %v", in, err)
	}
	if len(notes) != 1 || notes[0].Old != "env" || notes[0].New != "config env" || notes[0].RemovalAt != "v1.6.0" || notes[0].Kind != "move" {
		t.Fatalf("notes %+v", notes)
	}
	if tail := got[len(got)-3:]; !reflect.DeepEqual(tail, sp("prod -- env")) {
		t.Errorf("tail after the command words changed: %v", got)
	}
	// destructive old spellings: an unknown tail never selects another command's name
	for _, in := range []string{"env purge", "secrets rotate-all", "migrate reset", "ops nuke", "trust revoke-all"} {
		got, _, err := rewriteCanonArgsWith(&tb, root, sp(in), true)
		if err != nil {
			t.Fatal(err)
		}
		w := sp(in)
		if !strings.HasSuffix(strings.Join(got, " "), strings.Join(w[1:], " ")) {
			t.Errorf("%q: the unknown tail must survive unchanged, got %q", in, got)
		}
	}
	// an empty table rewrites nothing in either mode
	var empty canonTableT
	for _, v15 := range []bool{false, true} {
		if got, notes, err := rewriteCanonArgsWith(&empty, root, sp("env use"), v15); err != nil || len(notes) != 0 || strings.Join(got, " ") != "env use" {
			t.Errorf("empty table, v15=%v: %v %v %v", v15, got, notes, err)
		}
	}
	// the real generated table, whatever it holds, keeps unrelated argv intact
	for _, a := range []string{"status", "start", "version", "--help", "db migrate up"} {
		for _, v15 := range []bool{false, true} {
			if got, _, err := rewriteCanonArgs(sp(a), v15); err != nil || strings.Join(got, " ") != a {
				t.Errorf("%q v15=%v: %v %v", a, v15, got, err)
			}
		}
	}
}

func TestCanonEngineArgvV14Skip(t *testing.T) {
	tb, root := fixtureTable(t), newFixtureTree()
	var skipped, rewritten []string
	for _, r := range tb.Moves {
		probe := append(append([]string{}, r.To...), "arg")
		got, _, _ := rewriteCanonArgsWith(&tb, root, probe, false)
		if strings.Join(got, " ") == strings.Join(probe, " ") {
			skipped = append(skipped, strings.Join(r.To, " "))
		} else {
			rewritten = append(rewritten, strings.Join(r.To, " "))
		}
	}
	t.Logf("v1.4 skip list (new spellings that already resolve to a runnable command accepting the rest): %v", skipped)
	// deploy takes a target argument today, so `deploy ops` already means "deploy to ops"
	if want := []string{"deploy ops", "doctor heal", "update project run"}; !reflect.DeepEqual(sortedStrings(skipped), want) {
		t.Errorf("skip list = %v, want %v", skipped, want)
	}
	if len(rewritten) != len(tb.Moves)-3 {
		t.Errorf("both branches must be exercised: rewritten %v", rewritten)
	}
	// a skipped spelling keeps running today's command, in v1.4 only
	if got, _, _ := rewriteCanonArgsWith(&tb, root, sp("doctor heal now"), false); strings.Join(got, " ") != "doctor heal now" {
		t.Errorf("v1.4 doctor heal must run doctor: %v", got)
	}
	if got, _, _ := rewriteCanonArgsWith(&tb, root, sp("heal now"), true); strings.Join(got, " ") != "doctor heal now" {
		t.Errorf("v1.5 heal must move to doctor heal: %v", got)
	}
	// a spelling that resolves exactly to an existing command is never rewritten
	exact := canonTableT{Moves: []canonRowT{{From: sp("buy"), To: sp("doctor")}}}
	if got, _, _ := rewriteCanonArgsWith(&exact, root, sp("doctor"), false); strings.Join(got, " ") != "doctor" {
		t.Errorf("an existing command keeps its meaning: %v", got)
	}
	// the generated table, on the real tree (empty today): print what would be skipped
	reattachRealTree()
	n := 0
	for _, r := range canonTable.Moves {
		probe := append(append([]string{}, r.To...), "arg")
		if got, _, _ := rewriteCanonArgs(probe, false); strings.Join(got, " ") == strings.Join(probe, " ") {
			n++
			t.Logf("real v1.4 skip: nself %s", strings.Join(r.To, " "))
		}
	}
	t.Logf("real table: %d moves, %d skipped in v1.4", len(canonTable.Moves), n)
}

func sortedStrings(s []string) []string {
	out := append([]string{}, s...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// BenchmarkCanonArgv measures the per-invocation cost of the rewrite on a table
// of realistic size (every move Ticket together: about 150 rows) in both modes.
func BenchmarkCanonArgv(b *testing.B) {
	var tb canonTableT
	for i := 0; i < 150; i++ {
		w := "old" + strings.Repeat("x", i%7) + string(rune('a'+i%26))
		tb.Moves = append(tb.Moves, canonRowT{From: []string{w, "sub"}, To: []string{"hub", w}})
		tb.Shims = append(tb.Shims, canonRowT{From: []string{w + "s"}, To: []string{"stop"}})
	}
	root := newFixtureTree()
	args := sp("--json --home /tmp/x oldxxxxxxxg sub run --flag value")
	for _, v15 := range []bool{false, true} {
		name := "v14"
		if v15 {
			name = "v15"
		}
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, _ = rewriteCanonArgsWith(&tb, root, args, v15)
			}
		})
	}
}
