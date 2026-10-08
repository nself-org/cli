package canon

import (
	"bytes"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

const tableCanon = "schema_version: 1\nverbs: [stop, db]\n"

const tableFragment = `schema_version: 1
commands:
  config env: {side_effect: read}
  config vault: {side_effect: read}
  db: {canon: core, side_effect: read}
  stop: {canon: core, side_effect: write}
  plugin marketplace: {mode: v1.4, side_effect: read}
  service stop: {mode: v1.4, side_effect: write}
  runner: {mode: v1.4, canon: pending, side_effect: read}
  config: {canon: pending, side_effect: read}
hubs:
  - {path: db import, summary: "Import \"data\""}
moves:
  - {from: env, to: config env, since: v1.5.0, removal_at: v1.6.0}
  - {from: secrets, to: config vault, since: v1.5.0, removal_at: v1.6.0}
shims:
  - {from: service stop, to: stop, since: v1.5.0, removal_at: v1.6.0, equivalence: TestEquivalenceX}
retired_hubs:
  - {from: migrate, to: db, since: v1.5.0, removal_at: v1.6.0}
builtins: [completion]
breakouts:
  - {from: runner, plugin: ci, to: ci nodes, since: v1.5.0}
removed:
  - {from: plugin marketplace, since: v1.5.0, message: "no marketplace; see https://nself.org/plugins"}
`

func tableFile(t *testing.T) *File {
	t.Helper()
	f, err := LoadFS(fstest.MapFS{
		"canon.yaml":           {Data: []byte(tableCanon)},
		"domains/fixture.yaml": {Data: []byte(tableFragment)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestRenderGoTable(t *testing.T) {
	f := tableFile(t)
	a, b := RenderGoTable(f), RenderGoTable(f)
	if string(a) != string(b) {
		t.Fatal("rendering is not deterministic")
	}
	src := string(a)
	if !strings.HasPrefix(src, GeneratedMarker+"\n") {
		t.Errorf("missing the GENERATED marker:\n%s", src)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "table.go", a, 0); err != nil {
		t.Fatalf("not valid Go: %v\n%s", err, src)
	}
	for _, want := range []string{
		"package commands", "var canonTable = canonTableT{",
		`Verbs: []string{"stop", "db"}`,
		`{From: []string{"env"}, To: []string{"config", "env"}, Since: "v1.5.0", RemovalAt: "v1.6.0"}`,
		`{From: []string{"service", "stop"}, To: []string{"stop"}, Since: "v1.5.0", RemovalAt: "v1.6.0"}`,
		`{From: []string{"migrate"}, To: []string{"db"}`,
		`{From: []string{"db", "import"}, Summary: "Import \"data\""}`,
		`{From: []string{"runner"}, To: []string{"ci", "nodes"}, Since: "v1.5.0", Plugin: "ci"}`,
		`{From: []string{"plugin", "marketplace"}, Since: "v1.5.0", Message: "no marketplace; see https://nself.org/plugins"}`,
		`{"completion"}`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("rendered table lacks %s\n%s", want, src)
		}
	}
	// order: longest prefix first, then lexical
	tb := BuildTable(f)
	if got := strings.Join(tb.Moves[0].From, " ") + "|" + strings.Join(tb.Moves[1].From, " "); got != "env|secrets" {
		t.Errorf("moves order %s", got)
	}
	// every list renders only when it has rows, so an empty canon is a small valid file
	empty := RenderGoTable(&File{Verbs: []string{"stop"}})
	if _, err := parser.ParseFile(token.NewFileSet(), "e.go", empty, 0); err != nil || strings.Contains(string(empty), "Moves") {
		t.Errorf("empty canon: %v\n%s", err, empty)
	}
}

func TestRenderGoTableFormatted(t *testing.T) {
	f, err := LoadRaw()
	if err != nil {
		t.Fatal(err)
	}
	generated := RenderGoTable(f)
	formatted, err := format.Source(generated)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generated, formatted) {
		t.Fatal("generated Go differs from gofmt output")
	}
	if bytes.Contains(generated, []byte("\t\t[]string{")) {
		t.Fatal("generated builtin literals require formatter simplification")
	}
}

// TestCanonTableCurrent fails when cmd/commands/canon_moves_gen.go differs from
// what tools/canongen renders from the embedded canon (run `make canon`).
func TestCanonTableCurrent(t *testing.T) {
	f, err := LoadRaw()
	if err != nil {
		t.Fatal(err)
	}
	have, err := os.ReadFile("../../cmd/commands/canon_moves_gen.go")
	if err != nil {
		t.Fatalf("read the generated table: %v (run `make canon`)", err)
	}
	if !strings.HasPrefix(string(have), GeneratedMarker) {
		t.Error("canon_moves_gen.go lacks the GENERATED marker")
	}
	if want := RenderGoTable(f); string(have) != string(want) {
		t.Fatalf("cmd/commands/canon_moves_gen.go is stale; run `make canon` and commit the result.\nrendered:\n%s", want)
	}
	// mutation: a changed row must change the render (the comparison is not vacuous)
	f2 := *f
	f2.Moves = append(append([]Row{}, f.Moves...), Row{From: "zz-old", To: "zz new", Since: "v1.5.0", RemovalAt: "v1.6.0"})
	if string(RenderGoTable(&f2)) == string(have) {
		t.Error("adding a row did not change the rendered table")
	}
}
