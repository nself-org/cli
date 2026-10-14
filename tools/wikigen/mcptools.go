package main

// MCP-Tools.md: the tools `nself mcp` serves in v1.5 mode.
// Purpose: one row per tool of the committed tool list, so the page cannot
// name a tool the server does not serve or miss one it does.
// Inputs: the hermetic registry (no installed plugin), mcpgen.Tools in v1.5
// mode, the legacy-alias table and the committed tools golden (for the schema
// link line of each tool).
// Outputs: MCP-Tools.md; machine pages share writeMachinePages and writePage.
// Constraints: no hand text beyond the page frame; deterministic row order
// (canonical tools, then legacy aliases, each sorted by name).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/mcpgen"
	"github.com/nself-org/cli/schemas"
)

// machineSources are the committed inputs of the machine pages, relative to
// the repository root (wikigen runs there).
type machineSources struct{ golden, openapi, contract string }

var repoSources = machineSources{
	golden:   "internal/mcpgen/testdata/tools.golden.json",
	openapi:  ".github/openapi.json",
	contract: "internal/httpapi/contract.md",
}

const goldenLinkBase = "https://github.com/nself-org/cli/blob/main/internal/mcpgen/testdata/tools.golden.json"

// mcpRow is one tool of the page.
type mcpRow struct {
	Name, Summary, Side string
	Confirm             bool
	Line                int // line of the tool's inputSchema in the golden; 0 when unknown
}

// writeMachinePages writes (or, under check, compares) the three machine
// pages and returns the stale names under check, or the number written.
func writeMachinePages(dir string, reg *cmdregistry.Registry, src machineSources, check bool) (stale []string, written int, err error) {
	pages := []struct {
		name   string
		render func() (string, error)
	}{
		{"MCP-Tools.md", func() (string, error) { return renderMCPTools(reg, src.golden) }},
		{"HTTP-API-Reference.md", func() (string, error) { return renderHTTPAPI(reg, src) }},
		{"Agent-Guide.md", func() (string, error) { return renderAgentGuide(agentVerbs(reg)), nil }},
	}
	for _, p := range pages {
		content, err := p.render()
		if err != nil {
			return nil, 0, fmt.Errorf("%s: %w", p.name, err)
		}
		changed, err := writePage(filepath.Join(dir, p.name), content, check)
		if err != nil {
			return nil, 0, fmt.Errorf("%s: %w", p.name, err)
		}
		switch {
		case changed && check:
			stale = append(stale, p.name)
		case changed:
			written++
		}
	}
	return stale, written, nil
}

// writePage writes content unless the file already holds it; under check it
// only reports whether the file differs.
func writePage(path, content string, check bool) (bool, error) {
	if !pageStale(path, content) {
		return false, nil
	}
	if check {
		return true, nil
	}
	return true, os.WriteFile(path, []byte(content), 0o644)
}

// pluginCommandsParagraph is the closing line of both machine reference
// pages. It stays one line so the pages end with it.
func pluginCommandsParagraph(noun, rule string) string {
	return "**Plugin commands.** Installed plugins add " + noun + " by the same `" + rule +
		"` rule. This page is generated from the builtin registry alone, so it never lists them; each plugin's wiki page lists its own in its machine-surface region, for example the [CI plugin page](https://github.com/nself-org/plugins/wiki/CI).\n"
}

func renderMCPTools(reg *cmdregistry.Registry, goldenPath string) (string, error) {
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		return "", err
	}
	tools, err := mcpgen.Tools(mcpgen.Options{
		Registry: reg, V15: true, Schema: schemas.Lookup,
		Invoke: func(context.Context, string, invoke.Request) (invoke.Result, error) { return invoke.Result{}, nil },
	})
	if err != nil {
		return "", err
	}
	table, err := mcpgen.LoadTable()
	if err != nil {
		return "", err
	}
	canonical, aliases, err := mcpRows(tools, table, commandResolver(reg, table), schemaLines(golden))
	if err != nil {
		return "", err
	}
	return renderMCPPage(canonical, aliases, table.RemovalAt), nil
}

// commandResolver maps a tool name to the registry command behind it: the
// command whose ToolName it is, or the target of its legacy row.
func commandResolver(reg *cmdregistry.Registry, table *mcpgen.Table) func(string) *cmdregistry.Command {
	byName := map[string]*cmdregistry.Command{}
	for i := range reg.Commands {
		byName[cmdregistry.ToolName(reg.Commands[i].Path)] = &reg.Commands[i]
	}
	return func(name string) *cmdregistry.Command {
		if c, ok := byName[name]; ok {
			return c
		}
		if row, ok := table.Lookup(name); ok && row.Target != "" {
			c, _ := reg.Lookup(row.Target)
			return c
		}
		return nil
	}
}

// schemaLines reads the golden for the line of each tool's inputSchema. The
// golden is mcpgen's own indented output: tool keys sit at six spaces, and
// "inputSchema" precedes "name" within a tool.
func schemaLines(golden []byte) map[string]int {
	lines := map[string]int{}
	schemaAt := 0
	for i, l := range strings.Split(string(golden), "\n") {
		switch {
		case strings.HasPrefix(l, `      "inputSchema": `):
			schemaAt = i + 1
		case strings.HasPrefix(l, `      "name": "`):
			lines[strings.TrimSuffix(strings.TrimPrefix(l, `      "name": "`), `",`)] = schemaAt
		}
	}
	return lines
}

// mcpRows splits the tools into canonical tools and legacy aliases, sorted by
// name. A tool the golden does not list is an error: the page and the golden
// describe one tool list.
func mcpRows(tools []server.ServerTool, table *mcpgen.Table, resolve func(string) *cmdregistry.Command, lines map[string]int) (canonical, aliases []mcpRow, err error) {
	for _, st := range tools {
		t := st.Tool
		line, ok := lines[t.Name]
		if !ok {
			return nil, nil, fmt.Errorf("tool %s is not in the committed tools golden", t.Name)
		}
		legacy, isLegacy := table.Lookup(t.Name)
		isAlias := isLegacy && !legacy.Shortcut && legacy.Kind != mcpgen.KindMerged
		row := mcpRow{Name: t.Name, Summary: t.Description, Side: annotatedSide(t.Annotations), Line: line}
		cmd := resolve(t.Name)
		if cmd != nil {
			row.Confirm = cmd.Confirm != nil
			if !isAlias && !(isLegacy && legacy.Shortcut) { // aliases and shortcuts keep their own description
				row.Summary = cmd.Summary
			}
		}
		if isAlias {
			aliases = append(aliases, row)
		} else {
			canonical = append(canonical, row)
		}
	}
	for _, rows := range [][]mcpRow{canonical, aliases} {
		sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	}
	return canonical, aliases, nil
}

// annotatedSide inverts mcpgen's annotation mapping to the side-effect class.
func annotatedSide(a mcp.ToolAnnotation) string {
	set := func(b *bool) bool { return b != nil && *b }
	switch {
	case set(a.ReadOnlyHint):
		return canon.SideEffectRead
	case set(a.DestructiveHint):
		return canon.SideEffectDestructive
	case set(a.OpenWorldHint):
		return canon.SideEffectRemote
	}
	return canon.SideEffectWrite
}

func renderMCPPage(canonical, aliases []mcpRow, removalAt string) string {
	var b strings.Builder
	b.WriteString(generatedNotice + "\n# MCP Tools\n\n")
	b.WriteString("The tools `nself mcp` serves in v1.5 mode, one per exposed command of the machine registry, with no plugin installed. ")
	b.WriteString("Each tool takes `args`, `flags` and `confirm`; the input schema link opens the tool's entry in the committed tool list. ")
	b.WriteString("A tool that needs confirmation returns `confirmation_required` first; repeat the identical call with `confirm` set.\n\n")
	b.WriteString("## Tools\n\n")
	mcpTable(&b, canonical)
	b.WriteString("\n## Legacy aliases\n\n")
	b.WriteString("Deprecated names from v1.4, removed in " + removalAt + ". Each maps its parameters onto the canonical tool in its summary.\n\n")
	mcpTable(&b, aliases)
	b.WriteString("\n" + pluginCommandsParagraph("tools", "ToolName"))
	return b.String()
}

func mcpTable(b *strings.Builder, rows []mcpRow) {
	b.WriteString("| Name | Summary | Side effect | Confirm required | Input schema |\n|---|---|---|---|---|\n")
	for _, r := range rows {
		confirm := "no"
		if r.Confirm {
			confirm = "yes"
		}
		fmt.Fprintf(b, "| `%s` | %s | `%s` | %s | [input schema](%s#L%d) |\n", r.Name, cell(r.Summary), r.Side, confirm, goldenLinkBase, r.Line)
	}
}

// cell makes text safe inside one table cell.
func cell(s string) string {
	return strings.ReplaceAll(strings.Join(strings.Fields(s), " "), "|", `\|`)
}
