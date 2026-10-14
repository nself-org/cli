package main

// Tests for the machine pages: the MCP tool list against the committed
// golden, fixture drift for every input of -check, and the shared registry.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nself-org/cli/cmd/commands"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/mcpgen"
)

const (
	goldenFile   = "../../internal/mcpgen/testdata/tools.golden.json"
	openapiFile  = "../../.github/openapi.json"
	contractFile = "../../internal/httpapi/contract.md"
)

var repoTestSources = machineSources{golden: goldenFile, openapi: openapiFile, contract: contractFile}

var hermetic struct {
	once sync.Once
	reg  *cmdregistry.Registry
	err  error
}

// hermeticRegistry is the registry the wiki documents: v1.5, builtin only.
func hermeticRegistry(t testing.TB) *cmdregistry.Registry {
	t.Helper()
	hermetic.once.Do(func() {
		_, undo, err := prepareWikiRegistry()
		if err != nil {
			hermetic.err = err
			return
		}
		defer undo()
		hermetic.reg, hermetic.err = commands.BuildRegistry(true)
	})
	if hermetic.err != nil {
		t.Fatalf("wiki registry: %v", hermetic.err)
	}
	return hermetic.reg
}

var nameCell = regexp.MustCompile("^\\| `([a-z0-9_]+)` \\|")

func pageToolNames(page string) []string {
	var names []string
	for _, l := range strings.Split(page, "\n") {
		if m := nameCell.FindStringSubmatch(l); m != nil {
			names = append(names, m[1])
		}
	}
	sort.Strings(names)
	return names
}

func TestMCPToolsPageMatchesGolden(t *testing.T) {
	page, err := renderMCPTools(hermeticRegistry(t), goldenFile)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, tool := range golden.Tools {
		want = append(want, tool.Name)
	}
	sort.Strings(want)
	got := pageToolNames(page)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("page lists %d tools, golden has %d\npage:   %v\ngolden: %v", len(got), len(want), got, want)
	}
	if !strings.HasPrefix(page, generatedNotice+"\n") {
		t.Error("page lacks the generated header")
	}
	if !strings.Contains(page, "## Legacy aliases") || !strings.Contains(page, "Deprecated: use ") {
		t.Error("page lacks the legacy alias table")
	}
	if strings.Contains(page, "#L0)") {
		t.Error("a tool has no schema line")
	}
	if last := lastLine(page); !strings.HasPrefix(last, "**Plugin commands.**") {
		t.Errorf("last line is %q", last)
	}
}

func lastLine(page string) string {
	lines := strings.Split(strings.TrimRight(page, "\n"), "\n")
	return lines[len(lines)-1]
}

func fixtureTool(name, desc string, a mcp.ToolAnnotation) server.ServerTool {
	t := mcp.NewTool(name, mcp.WithDescription(desc))
	t.Annotations = a
	return server.ServerTool{Tool: t}
}

func TestMCPRowsFixture(t *testing.T) {
	table, err := mcpgen.LoadTable()
	if err != nil {
		t.Fatal(err)
	}
	confirm := &cmdregistry.Command{Summary: "Remove it", SideEffect: "destructive", Confirm: &cmdregistry.Confirm{}}
	resolve := func(name string) *cmdregistry.Command {
		if name == "nself_fixture_destroy" {
			return confirm
		}
		return nil
	}
	tools := []server.ServerTool{
		fixtureTool("nself_urls", "Deprecated: use nself_status_urls; removed in v1.6.0. Show URLs.", mcp.ToolAnnotation{ReadOnlyHint: mcp.ToBoolPtr(true)}),
		fixtureTool("nself_fixture_destroy", "Remove it. Destructive.", mcp.ToolAnnotation{DestructiveHint: mcp.ToBoolPtr(true)}),
		fixtureTool("nself_fixture_a", "Reads a | b.", mcp.ToolAnnotation{ReadOnlyHint: mcp.ToBoolPtr(true)}),
	}
	lines := map[string]int{"nself_urls": 7, "nself_fixture_destroy": 40, "nself_fixture_a": 80}
	canonical, aliases, err := mcpRows(tools, table, resolve, lines)
	if err != nil {
		t.Fatal(err)
	}
	if len(canonical) != 2 || canonical[0].Name != "nself_fixture_a" || canonical[1].Name != "nself_fixture_destroy" {
		t.Fatalf("canonical rows: %+v", canonical)
	}
	if len(aliases) != 1 || aliases[0].Name != "nself_urls" {
		t.Fatalf("alias rows: %+v", aliases)
	}
	if canonical[1].Confirm != "yes" || canonical[1].Side != "destructive" || canonical[1].Summary != "Remove it" || canonical[0].Confirm != "no" {
		t.Errorf("destroy row: %+v", canonical[1])
	}
	page := renderMCPPage(canonical, aliases, table.RemovalAt)
	for _, want := range []string{
		"| `nself_fixture_a` | Reads a \\| b. | `read` | no | [input schema](https://github.com/nself-org/cli/blob/main/internal/mcpgen/testdata/tools.golden.json#L80) |",
		"| `nself_fixture_destroy` | Remove it | `destructive` | yes |",
		"| `nself_urls` | Deprecated: use nself_status_urls",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if _, _, err := mcpRows(tools, table, resolve, map[string]int{}); err == nil {
		t.Error("a tool missing from the golden must fail")
	}
}

func TestSchemaLines(t *testing.T) {
	golden := "{\n  \"tools\": [\n    {\n      \"inputSchema\": {\n        \"name\": \"inner\"\n      },\n      \"name\": \"nself_x\",\n      \"outputSchema\": {}\n    }\n  ]\n}\n"
	got := schemaLines([]byte(golden))
	if len(got) != 1 || got["nself_x"] != 4 {
		t.Errorf("schemaLines = %v, want nself_x at 4", got)
	}
}

// copySources copies the committed inputs into dir so a test can change one.
func copySources(t *testing.T, dir string) machineSources {
	t.Helper()
	out := machineSources{golden: filepath.Join(dir, "tools.golden.json"), openapi: filepath.Join(dir, "openapi.json"), contract: filepath.Join(dir, "contract.md")}
	for from, to := range map[string]string{goldenFile: out.golden, openapiFile: out.openapi, contractFile: out.contract} {
		b, err := os.ReadFile(from)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(to, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// TestMachinePagesCheckDrift: -check passes on written pages and fails when
// the tool list golden, the OpenAPI document or the contract text changes.
func TestMachinePagesCheckDrift(t *testing.T) {
	reg := hermeticRegistry(t)
	dir := t.TempDir()
	src := copySources(t, dir)
	if _, written, err := writeMachinePages(dir, reg, src, false); err != nil || written != 3 {
		t.Fatalf("write: written=%d err=%v", written, err)
	}
	if stale, _, err := writeMachinePages(dir, reg, src, true); err != nil || len(stale) != 0 {
		t.Fatalf("fresh pages reported stale: %v %v", stale, err)
	}
	edit := func(path, old, repl string) {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), old) {
			t.Fatalf("%s lacks %q", path, old)
		}
		if err := os.WriteFile(path, []byte(strings.Replace(string(b), old, repl, 1)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	edit(src.golden, "{\n", "{\n\n") // shifts every schema link line
	edit(src.openapi, `"summary": "Read this OpenAPI document"`, `"summary": "Read the document"`)
	edit(src.contract, "# CLI HTTP API v1", "# CLI HTTP API v2")
	stale, _, err := writeMachinePages(dir, reg, src, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(stale, ",") != "MCP-Tools.md,HTTP-API-Reference.md" {
		t.Errorf("stale after fixture edits = %v", stale)
	}
}

func sp(s string) *string { return &s }

// TestConfirmCell: the Confirm required column follows the invoker's gate.
func TestConfirmCell(t *testing.T) {
	gate := &cmdregistry.Confirm{Flags: []string{"yes"}}
	planGate := &cmdregistry.Confirm{Flags: []string{"yes"}, Plan: &cmdregistry.PlanForm{Flag: "plan", IDFlag: "plan-id"}}
	all := cmdregistry.Flag{Name: "all", Type: "bool", SideEffect: sp("destructive")}
	apply := cmdregistry.Flag{Name: "apply", Type: "bool", SideEffect: sp("write")}
	cases := []struct {
		name  string
		cmd   cmdregistry.Command
		only  map[string]bool
		wantC string
	}{
		{"read", cmdregistry.Command{SideEffect: "read"}, nil, "no"},
		{"read plan form never asks", cmdregistry.Command{SideEffect: "read", Confirm: planGate}, nil, "no"},
		{"write without plan", cmdregistry.Command{SideEffect: "write"}, nil, "no"},
		{"write with plan", cmdregistry.Command{SideEffect: "write", Confirm: planGate}, nil, "if the plan requires it"},
		{"remote", cmdregistry.Command{SideEffect: "remote", Confirm: gate}, nil, "yes"},
		{"destructive plan form", cmdregistry.Command{SideEffect: "destructive", Confirm: planGate}, nil, "yes"},
		{"normal clean, --all destructive", cmdregistry.Command{SideEffect: "write", Confirm: gate, Flags: []cmdregistry.Flag{all}}, nil, "with `--all`"},
		{"read, --apply writes under a plan", cmdregistry.Command{SideEffect: "read", Confirm: planGate, Flags: []cmdregistry.Flag{apply}}, nil, "with `--apply` if the plan requires it"},
		{"write plan and --all", cmdregistry.Command{SideEffect: "write", Confirm: planGate, Flags: []cmdregistry.Flag{all}}, nil, "if the plan requires it; with `--all`"},
		{"read, --apply writes without a plan", cmdregistry.Command{SideEffect: "read", Flags: []cmdregistry.Flag{apply}}, nil, "no"},
		{"alias maps no flags", cmdregistry.Command{SideEffect: "write", Confirm: gate, Flags: []cmdregistry.Flag{all}}, map[string]bool{}, "no"},
		{"alias maps --all", cmdregistry.Command{SideEffect: "write", Confirm: gate, Flags: []cmdregistry.Flag{all}}, map[string]bool{"all": true}, "with `--all`"},
		{"escalating flag not exposed", cmdregistry.Command{SideEffect: "write", Confirm: gate, Flags: []cmdregistry.Flag{
			{Name: "all", Type: "bool", SideEffect: sp("destructive"), Hidden: true}}}, nil, "no"},
		{"escalating flag owned by the invoker", cmdregistry.Command{SideEffect: "write", Confirm: gate, Flags: []cmdregistry.Flag{
			{Name: "yes", Type: "bool", SideEffect: sp("destructive")}}}, nil, "no"},
		{"escalating flag without a gate is refused, not asked", cmdregistry.Command{SideEffect: "write", Flags: []cmdregistry.Flag{all}}, nil, "no"},
	}
	for _, c := range cases {
		cmd := c.cmd
		if got := confirmCell(&cmd, c.only); got != c.wantC {
			t.Errorf("%s: confirmCell = %q, want %q", c.name, got, c.wantC)
		}
	}
}

// fixtureGolden renders tools as a golden document.
func fixtureGolden(t *testing.T, tools []server.ServerTool) []byte {
	t.Helper()
	list := make([]json.RawMessage, 0, len(tools))
	for _, st := range tools {
		b, err := json.Marshal(st.Tool)
		if err != nil {
			t.Fatal(err)
		}
		list = append(list, b)
	}
	b, err := json.MarshalIndent(map[string]any{"tools": list}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestCheckGoldenSemantic: any change to a tool or to the tool set is an
// error even when the golden keeps its line layout.
func TestCheckGoldenSemantic(t *testing.T) {
	mk := func(desc string, ro bool) server.ServerTool {
		st := fixtureTool("nself_fixture_a", desc, mcp.ToolAnnotation{ReadOnlyHint: mcp.ToBoolPtr(ro)})
		st.Tool.InputSchema = mcp.ToolInputSchema{Type: "object", Properties: map[string]any{"mode": map[string]any{"type": "string"}}}
		return st
	}
	base := []server.ServerTool{mk("Reads a.", true), fixtureTool("nself_fixture_b", "Reads b.", mcp.ToolAnnotation{})}
	golden := fixtureGolden(t, base)
	if err := checkGolden(base, golden); err != nil {
		t.Fatalf("identical tools: %v", err)
	}
	mutate := func(old, repl string) []byte {
		if !strings.Contains(string(golden), old) {
			t.Fatalf("golden lacks %q", old)
		}
		return []byte(strings.Replace(string(golden), old, repl, 1))
	}
	for name, tc := range map[string]struct {
		golden []byte
		want   string
	}{
		"description":  {mutate("Reads a.", "Reads z."), "description"},
		"input schema": {mutate(`"type": "string"`, `"type": "number"`), "inputSchema"},
		"annotation":   {mutate(`"readOnlyHint": true`, `"readOnlyHint": false`), "annotations"},
		"tool removed": {fixtureGolden(t, base[:1]), "nself_fixture_b: generated but not in the golden"},
		"tool added":   {fixtureGolden(t, append(append([]server.ServerTool{}, base...), fixtureTool("nself_fixture_c", "c", mcp.ToolAnnotation{}))), "nself_fixture_c: in the golden but not generated"},
	} {
		if same := len(strings.Split(string(tc.golden), "\n")) == len(strings.Split(string(golden), "\n")); name != "tool removed" && name != "tool added" && !same {
			t.Errorf("%s: mutation changed the line count", name)
		}
		err := checkGolden(base, tc.golden)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, tc.want)
		}
	}
}

// TestMachinePagesSemanticAndFooterDrift: a same-line change of the golden
// fails generation and -check; a changed footer makes a reference page stale.
func TestMachinePagesSemanticAndFooterDrift(t *testing.T) {
	reg := hermeticRegistry(t)
	dir := t.TempDir()
	src := copySources(t, dir)
	if _, _, err := writeMachinePages(dir, reg, src, false); err != nil {
		t.Fatal(err)
	}
	for _, page := range []string{"MCP-Tools.md", "HTTP-API-Reference.md"} {
		path := filepath.Join(dir, page)
		orig, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for name, tampered := range map[string]string{
			"footer text changed": strings.Replace(string(orig), "never lists them", "lists them", 1),
			"footer removed":      strings.TrimRight(string(orig), "\n")[:strings.LastIndex(strings.TrimRight(string(orig), "\n"), "\n")+1],
			"footer link changed": strings.Replace(string(orig), "/wiki/CI)", "/wiki/Other)", 1),
		} {
			if tampered == string(orig) {
				t.Fatalf("%s %s: tamper did not change the page", page, name)
			}
			if err := os.WriteFile(path, []byte(tampered), 0o644); err != nil {
				t.Fatal(err)
			}
			stale, _, err := writeMachinePages(dir, reg, src, true)
			if err != nil || len(stale) != 1 || stale[0] != page {
				t.Errorf("%s %s: stale=%v err=%v", page, name, stale, err)
			}
		}
		if err := os.WriteFile(path, orig, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	golden, err := os.ReadFile(src.golden)
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(golden), `"idempotentHint": true`, `"idempotentHint": false`, 1)
	if mutated == string(golden) {
		t.Fatal("golden has no idempotentHint to change")
	}
	if err := os.WriteFile(src.golden, []byte(mutated), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := writeMachinePages(dir, reg, src, true); err == nil || !strings.Contains(err.Error(), "annotations") {
		t.Errorf("same-line golden change: err = %v, want a golden mismatch", err)
	}
}

// TestCheckGoldenAmbiguity: a duplicated, unnamed or null-padded golden entry
// is rejected, though each keeps the schema anchors of the existing tools.
func TestCheckGoldenAmbiguity(t *testing.T) {
	base := []server.ServerTool{
		fixtureTool("nself_fixture_a", "Reads a.", mcp.ToolAnnotation{ReadOnlyHint: mcp.ToBoolPtr(true)}),
		fixtureTool("nself_fixture_b", "Reads b.", mcp.ToolAnnotation{}),
	}
	golden := fixtureGolden(t, base)
	lines := len(strings.Split(string(golden), "\n"))
	mutate := func(old, repl string) []byte {
		if !strings.Contains(string(golden), old) {
			t.Fatalf("golden lacks %q", old)
		}
		return []byte(strings.Replace(string(golden), old, repl, 1))
	}
	dup := fixtureGolden(t, append(append([]server.ServerTool{}, base...), base[1]))
	anchors, dupAnchors := schemaLines(golden), schemaLines(dup)
	for name, line := range anchors {
		if dupAnchors[name] != line {
			t.Fatalf("duplicate fixture moved the anchor of %s", name)
		}
	}
	for name, tc := range map[string]struct {
		golden []byte
		want   string
	}{
		"duplicate final tool": {dup, "repeated name"},
		"empty name":           {mutate(`"name": "nself_fixture_b"`, `"name": ""`), "empty or repeated name"},
		"missing name":         {mutate(`"name": "nself_fixture_b"`, `"label": "nself_fixture_b"`), "empty or repeated name"},
		"explicit null field":  {mutate(`"name": "nself_fixture_a"`, `"name": "nself_fixture_a", "extra": null`), "extra"},
	} {
		if name != "duplicate final tool" && len(strings.Split(string(tc.golden), "\n")) != lines {
			t.Errorf("%s: mutation changed the line count", name)
		}
		if err := checkGolden(base, tc.golden); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, tc.want)
		}
	}
}

// TestMachinePagesRejectAmbiguousGolden: -check fails on the real golden with
// a duplicated final tool or an extra null field, existing anchors unchanged.
func TestMachinePagesRejectAmbiguousGolden(t *testing.T) {
	reg := hermeticRegistry(t)
	dir := t.TempDir()
	src := copySources(t, dir)
	if _, _, err := writeMachinePages(dir, reg, src, false); err != nil {
		t.Fatal(err)
	}
	orig, err := os.ReadFile(src.golden)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(orig, &doc); err != nil {
		t.Fatal(err)
	}
	doc.Tools = append(doc.Tools, doc.Tools[len(doc.Tools)-1])
	dup, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		golden string
		want   string
	}{
		"duplicate final tool": {string(dup) + "\n", "repeated name"},
		"explicit null field":  {strings.Replace(string(orig), `"name": "nself_backup",`, `"name": "nself_backup", "extra": null,`, 1), "extra"},
	} {
		if tc.golden == string(orig) {
			t.Fatalf("%s: no change", name)
		}
		if name == "duplicate final tool" {
			before, after := schemaLines(orig), schemaLines([]byte(tc.golden))
			for tool, line := range before {
				if after[tool] != line {
					t.Fatalf("%s moved the anchor of %s", name, tool)
				}
			}
		}
		if err := os.WriteFile(src.golden, []byte(tc.golden), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := writeMachinePages(dir, reg, src, true); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, tc.want)
		}
	}
}
