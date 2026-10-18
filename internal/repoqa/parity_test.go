package repoqa

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
)

// TestParityMatrixIsCurrent fails when .github/surface-parity.{md,json} no
// longer match tools/parity's output (CLI-R17), the same drift guard
// TestCommandInventoryIsCurrent applies to the command inventory. The matrix
// is built from that same committed inventory, so this test only asserts the
// matrix itself is current — it does not re-derive the command list.
func TestParityMatrixIsCurrent(t *testing.T) {
	root := repoRoot(t)

	gen := exec.Command("go", "run", "-mod=vendor", "./tools/parity", "-check")
	gen.Dir = root
	gen.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := gen.CombinedOutput()
	if err != nil {
		t.Fatalf("parity matrix is stale — run `make parity` and commit the result:\n%s", out)
	}
}

// TestParityMatrixCommandsHaveWikiPages is a direct content assertion on the
// committed matrix, independent of the -check regeneration above: every
// top-level command it lists must be marked wiki_page: true. Docs are the
// minimum bar (CLI-R17) — this is the hard gate; MCP tool coverage, env var
// docs, and OpenAPI routes are informational only (see .github/surface-parity.md
// header for why).
func TestParityMatrixCommandsHaveWikiPages(t *testing.T) {
	root := repoRoot(t)

	data, err := os.ReadFile(filepath.Join(root, ".github", "surface-parity.json"))
	if err != nil {
		t.Fatalf("read surface-parity.json: %v (run `make parity`)", err)
	}

	var doc struct {
		Rows []struct {
			Path     string `json:"path"`
			WikiPage bool   `json:"wiki_page"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse surface-parity.json: %v", err)
	}
	if len(doc.Rows) == 0 {
		t.Fatal("surface-parity.json has no rows")
	}
	for _, r := range doc.Rows {
		if !r.WikiPage {
			t.Errorf("%s has no wiki page (wiki_page: false in surface-parity.json)", r.Path)
		}
	}
}

// parityRow is the part of a surface-parity.json row the registry check reads.
type parityRow struct {
	Name       string `json:"name"`
	MCPTool    bool   `json:"mcp_tool"`
	MCPTools   int    `json:"mcp_tools"`
	HTTPRoute  bool   `json:"http_route"`
	HTTPRoutes int    `json:"http_routes"`
}

// parityDiffs recomputes the MCP and HTTP columns from the committed registry,
// the committed tool golden (tool names) and the committed OpenAPI (route
// paths), none of which tools/parity reads, and lists every row that differs.
func parityDiffs(rows []parityRow, reg *cmdregistry.Registry, tools, routes map[string]bool) []string {
	want := map[string]*parityRow{}
	for i := range reg.Commands {
		cmd := &reg.Commands[i]
		top, _, _ := strings.Cut(strings.TrimPrefix(cmd.Path, "nself "), " ")
		w := want[top]
		if w == nil {
			w = &parityRow{Name: top}
			want[top] = w
		}
		if ok, _ := invoke.Exposure(cmd, invoke.TransportMCP, nil); ok && tools[cmdregistry.ToolName(cmd.Path)] {
			w.MCPTools++
		}
		if ok, _ := invoke.Exposure(cmd, invoke.TransportHTTP, nil); ok && routes[cmdregistry.RoutePath(cmd.Path)] {
			w.HTTPRoutes++
		}
	}
	var diffs []string
	for _, r := range rows {
		w := want[r.Name]
		switch {
		case w == nil:
			diffs = append(diffs, fmt.Sprintf("%s: no such top-level command in the registry", r.Name))
		case r.MCPTools != w.MCPTools || r.MCPTool != (w.MCPTools > 0):
			diffs = append(diffs, fmt.Sprintf("%s: matrix says %d MCP tools (mcp_tool %v), registry and golden say %d", r.Name, r.MCPTools, r.MCPTool, w.MCPTools))
		case r.HTTPRoutes != w.HTTPRoutes || r.HTTPRoute != (w.HTTPRoutes > 0):
			diffs = append(diffs, fmt.Sprintf("%s: matrix says %d HTTP routes (http_route %v), registry and OpenAPI say %d", r.Name, r.HTTPRoutes, r.HTTPRoute, w.HTTPRoutes))
		}
	}
	return diffs
}

// readsHandSources reports whether Go source scans the hand-written MCP files
// as text, the way tools/parity and tools/mcpdoc did before P7-SURF-23.
func readsHandSources(src string) bool {
	return regexp.MustCompile(`cmd/commands/mcp[a-z_]*\.go|mcp\.NewTool`).MatchString(src)
}

// TestParityFromRegistry: tools/parity reads the registry, not mcp.go. The
// MCP and HTTP columns of the committed matrix equal what the committed
// registry, tool golden and OpenAPI say; no generator text-scans the hand MCP
// files; tools/mcpdoc and its make target are gone (P7-SURF-23).
func TestParityFromRegistry(t *testing.T) {
	root := repoRoot(t)
	read := func(rel string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}

	var reg cmdregistry.Registry
	if err := json.Unmarshal(read(".github/command-registry.json"), &reg); err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(read("internal/mcpgen/testdata/tools.golden.json"), &golden); err != nil {
		t.Fatal(err)
	}
	tools := map[string]bool{}
	for _, tool := range golden.Tools {
		tools[tool.Name] = true
	}
	var doc struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(read(".github/openapi.json"), &doc); err != nil {
		t.Fatal(err)
	}
	routes := map[string]bool{}
	for p := range doc.Paths {
		routes[p] = true
	}
	var matrix struct {
		Rows []parityRow `json:"rows"`
	}
	if err := json.Unmarshal(read(".github/surface-parity.json"), &matrix); err != nil {
		t.Fatal(err)
	}
	if len(matrix.Rows) < 10 || len(tools) < 50 || len(routes) < 50 {
		t.Fatalf("vacuous inputs: %d rows, %d tools, %d routes", len(matrix.Rows), len(tools), len(routes))
	}
	for _, d := range parityDiffs(matrix.Rows, &reg, tools, routes) {
		t.Error(d)
	}

	// Negative: a matrix that claims one tool too many for the first row must differ.
	forged := append([]parityRow{}, matrix.Rows...)
	forged[0].MCPTools++
	forged[0].MCPTool = true
	if len(parityDiffs(forged, &reg, tools, routes)) == 0 {
		t.Error("a tampered MCP count was not detected")
	}

	// No generator reads the hand MCP files as text, and the old pieces are gone.
	if !readsHandSources(`os.ReadFile("cmd/commands/mcp_sentry.go")`) || readsHandSources("package main") {
		t.Fatal("the hand-source detector is wrong")
	}
	matches, err := filepath.Glob(filepath.Join(root, "tools", "parity", "*.go"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no tools/parity sources: %v", err)
	}
	for _, f := range matches {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		if readsHandSources(string(read(filepath.Join("tools", "parity", filepath.Base(f))))) {
			t.Errorf("%s scans the hand MCP sources", f)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "tools", "mcpdoc")); !os.IsNotExist(err) {
		t.Errorf("tools/mcpdoc still exists (stat err %v)", err)
	}
	if strings.Contains(string(read("Makefile")), "mcp-docs") {
		t.Error("the Makefile still has the mcp-docs target")
	}
}
