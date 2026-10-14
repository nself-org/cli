package main

// MCP-Tools.md: the tools `nself mcp` serves in v1.5 mode.
// Purpose: one row per tool of the committed tool list, so the page cannot
// name a tool the server does not serve or miss one it does.
// Inputs: the hermetic registry, mcpgen.Tools in v1.5 mode, the legacy-alias
// table and the committed tools golden (schema link line of each tool).
// Outputs: MCP-Tools.md (writeMachinePages is in agentguide.go).
// Constraints: no hand text beyond the page frame; rows sorted by name,
// canonical tools then legacy aliases. The generated tools must equal the
// committed golden semantically (checkGolden) or generation fails.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
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

const goldenLinkBase = "https://github.com/nself-org/cli/blob/main/internal/mcpgen/testdata/tools.golden.json"

// mcpRow is one tool of the page.
type mcpRow struct {
	Name, Summary, Side string
	Confirm             string // see confirmCell
	Line                int    // line of the tool's inputSchema in the golden; 0 when unknown
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
	if err := checkGolden(tools, golden); err != nil {
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
		row.Confirm = "no"
		if cmd != nil {
			var only map[string]bool // an alias reaches only the flags its row maps
			if isAlias {
				only = map[string]bool{}
				for _, f := range legacy.Flags {
					only[f.Flag] = true
				}
			}
			row.Confirm = confirmCell(cmd, only)
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
	on := func(b *bool) bool { return b != nil && *b }
	switch {
	case on(a.ReadOnlyHint):
		return canon.SideEffectRead
	case on(a.DestructiveHint):
		return canon.SideEffectDestructive
	case on(a.OpenWorldHint):
		return canon.SideEffectRemote
	}
	return canon.SideEffectWrite
}

func renderMCPPage(canonical, aliases []mcpRow, removalAt string) string {
	var b strings.Builder
	b.WriteString(generatedNotice + "\n# MCP Tools\n\nThe tools `nself mcp` serves in v1.5 mode, one per exposed command of the machine registry, with no plugin installed. ")
	b.WriteString("Each tool takes `args`, `flags` and `confirm`; the input schema link opens the tool's entry in the committed tool list. ")
	b.WriteString("A call that needs confirmation returns `confirmation_required` first; repeat the identical call with `confirm` set. " +
		"Confirm required follows the invoker's gate: `yes` for a remote or destructive command, `no` for a read command or a write command with no plan form, ")
	b.WriteString("`with --flag` when only that exposed flag raises the side effect to remote or destructive, and `if the plan requires it` for a write command with a plan form, which asks only when its plan says so.\n\n## Tools\n\n")
	mcpTable(&b, canonical)
	b.WriteString("\n## Legacy aliases\n\nDeprecated names from v1.4, removed in " + removalAt + ". Each maps its parameters onto the canonical tool in its summary.\n\n")
	mcpTable(&b, aliases)
	b.WriteString("\n" + pluginCommandsParagraph("tools", "ToolName"))
	return b.String()
}

func mcpTable(b *strings.Builder, rows []mcpRow) {
	b.WriteString("| Name | Summary | Side effect | Confirm required | Input schema |\n|---|---|---|---|---|\n")
	for _, r := range rows {
		fmt.Fprintf(b, "| `%s` | %s | `%s` | %s | [input schema](%s#L%d) |\n", r.Name, cell(r.Summary), r.Side, cell(r.Confirm), goldenLinkBase, r.Line)
	}
}

// confirmCell says when a call to cmd asks for confirmation, from the
// invoker's gate (internal/invoke Gate and planGate): a remote or destructive
// effective side effect asks (the registry's confirm block makes the command
// exposed), read never asks, write asks only when its command has a plan form
// and the plan requires it. A flag asks when it raises the side effect above
// the command's own. only, when set, limits the flags to those a legacy alias
// maps; flags the machine request cannot carry (invoke.ExposedFlags) are out.
func confirmCell(cmd *cmdregistry.Command, only map[string]bool) string {
	plan := cmd.Confirm != nil && cmd.Confirm.Plan != nil
	level := func(side string) string {
		switch side {
		case canon.SideEffectRemote, canon.SideEffectDestructive:
			if cmd.Confirm != nil {
				return "yes"
			}
		case canon.SideEffectWrite:
			if plan {
				return "plan"
			}
		}
		return "no"
	}
	base := level(cmd.SideEffect)
	if base == "yes" {
		return "yes"
	}
	var yes, planned, parts []string
	for _, f := range invoke.ExposedFlags(cmd, invoke.TransportMCP) {
		if (only != nil && !only[f.Name]) || f.SideEffect == nil || canon.Rank(*f.SideEffect) <= canon.Rank(cmd.SideEffect) {
			continue
		}
		switch level(*f.SideEffect) {
		case "yes":
			yes = append(yes, "`--"+f.Name+"`")
		case "plan":
			if base != "plan" {
				planned = append(planned, "`--"+f.Name+"`")
			}
		}
	}
	if base == "plan" {
		parts = append(parts, "if the plan requires it")
	}
	if len(planned) > 0 {
		parts = append(parts, "with "+strings.Join(planned, ", ")+" if the plan requires it")
	}
	if len(yes) > 0 {
		parts = append(parts, "with "+strings.Join(yes, ", "))
	}
	if len(parts) == 0 {
		return "no"
	}
	return strings.Join(parts, "; ")
}

// checkGolden compares the generated tools with the complete committed golden
// by value (key order and layout do not matter): a changed description,
// input or output schema, annotation, an added, removed, duplicated or
// unnamed tool, or a key present on one side only (even as null) is an error.
func checkGolden(tools []server.ServerTool, golden []byte) error {
	var doc struct {
		Tools []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(golden, &doc); err != nil {
		return fmt.Errorf("tools golden: %w", err)
	}
	want := map[string]map[string]any{}
	for i, t := range doc.Tools {
		name, _ := t["name"].(string)
		if _, dup := want[name]; dup || name == "" {
			return fmt.Errorf("tools golden: tool %d has an empty or repeated name %q", i, name)
		}
		want[name] = t
	}
	var problems []string
	for _, st := range tools {
		name, got := st.Tool.Name, map[string]any{}
		raw, err := json.Marshal(st.Tool)
		if err == nil {
			err = json.Unmarshal(raw, &got)
		}
		if err != nil {
			return fmt.Errorf("tool %s: %w", name, err)
		}
		w, ok := want[name]
		delete(want, name)
		switch {
		case !ok:
			problems = append(problems, name+": generated but not in the golden")
		case !reflect.DeepEqual(got, w): // whole maps: an absent key is not an explicit null
			problems = append(problems, name+": differs from the golden in "+strings.Join(differingKeys(got, w), ", "))
		}
	}
	for name := range want {
		problems = append(problems, name+": in the golden but not generated")
	}
	if len(problems) == 0 {
		return nil
	}
	if sort.Strings(problems); len(problems) > 10 {
		problems = append(problems[:10], "...")
	}
	return fmt.Errorf("the generated MCP tools differ from internal/mcpgen/testdata/tools.golden.json (run go test ./internal/mcpgen/ -run TestToolListGolden -update, review, then wikigen):\n  %s", strings.Join(problems, "\n  "))
}

// differingKeys lists the keys whose presence or value differs, sorted.
func differingKeys(a, b map[string]any) []string {
	var out []string
	for k, av := range a {
		if bv, ok := b[k]; !ok || !reflect.DeepEqual(av, bv) {
			out = append(out, k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
