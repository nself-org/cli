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
	confirm := &cmdregistry.Command{Summary: "Remove it", Confirm: &cmdregistry.Confirm{}}
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
	if !canonical[1].Confirm || canonical[1].Side != "destructive" || canonical[1].Summary != "Remove it" || canonical[0].Confirm {
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
