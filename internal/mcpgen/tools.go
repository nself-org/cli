// Package mcpgen generates the MCP surface from the machine registry
// (contract:cli.mcp-tools v2, EPIC P7-SURF D5).
//
// Purpose: one MCP tool per exposed document command of the registry (core,
// subcommand and installed plugin commands), so the tool list is data, not
// code. The input schema is the machine request schema, the output schema is
// the command's committed data schema, annotations come from the side effect,
// and every call runs through the invoker and its gate.
//
// Inputs: a *cmdregistry.Registry, an Invoker, a schema lookup, the compat mode.
//
// Outputs: server.ServerTool values (Tools), resources (Resources) and server
// options (ServerOptions).
//
// Constraints: v1.4 mode keeps the 26 legacy tools primary and adds generated
// tools only under names no legacy tool uses; v1.5 mode registers generated
// tools only, where the 13 legacy names equal to a generated name become that
// one tool with a dual-shape input and the other 13 are deprecated aliases
// (legacy_aliases.yaml). No name is ever registered twice. This package imports
// invoke, cmdregistry, canon, errs, output and mcp-go, never internal/plugin.
//
// Layer: L2.
package mcpgen

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
)

// Invoker runs one registry command; cmd/commands wires invoke.Invoke with the
// project directory, the confirmation Guard and the audit surface.
type Invoker func(ctx context.Context, path string, req invoke.Request) (invoke.Result, error)

// Options configures Tools.
type Options struct {
	Registry *cmdregistry.Registry
	Invoke   Invoker
	// Schema returns a committed schema by its path relative to schemas/.
	Schema func(name string) ([]byte, bool)
	// V15 selects v1.5 mode: generated tools and aliases only.
	V15 bool
	// Warn receives one line for each plugin command skipped because its tool
	// name is invalid or taken; core conflicts fail Tools instead.
	Warn func(string)
}

type gen struct {
	Options
	table *Table
}

// Tools builds the tools for the mode, sorted by name.
func Tools(o Options) ([]server.ServerTool, error) {
	if o.Registry == nil || o.Invoke == nil {
		return nil, fmt.Errorf("mcpgen: Tools needs a registry and an invoker")
	}
	table, err := LoadTable()
	if err != nil {
		return nil, err
	}
	g := &gen{Options: o, table: table}
	byName := map[string]server.ServerTool{}
	paths := map[string]string{}
	add := func(name, path string, t server.ServerTool) error {
		if prev, dup := paths[name]; dup {
			return fmt.Errorf("mcpgen: tool name %q is registered twice (%s and %s)", name, prev, path)
		}
		paths[name] = path
		byName[name] = t
		return nil
	}
	merged := map[string]bool{}
	for i := range o.Registry.Commands {
		cmd := &o.Registry.Commands[i]
		if ok, _ := invoke.Exposure(cmd, invoke.TransportMCP, nil); !ok {
			continue
		}
		name := cmdregistry.ToolName(cmd.Path)
		if len(name) > 64 {
			if skipPlugin(o, cmd, "tool name is longer than 64 characters") {
				continue
			}
			return nil, fmt.Errorf("mcpgen: tool name %q for %q is longer than 64 characters", name, cmd.Path)
		}
		row, legacy := table.Lookup(name)
		legacy = legacy && !row.Shortcut
		switch {
		case legacy && !o.V15:
			continue // a hand tool owns the name in v1.4 mode
		case legacy && row.Kind != KindMerged:
			if skipPlugin(o, cmd, "its tool name is a legacy alias name") {
				continue
			}
			return nil, fmt.Errorf("mcpgen: generated name %q is also the %s legacy name of %q", name, row.Kind, row.Target)
		case legacy && barePath(cmd.Path) != row.Target:
			return nil, fmt.Errorf("mcpgen: merged row %s targets %q but the generated tool is %q", name, row.Target, cmd.Path)
		}
		if !legacy {
			row = nil
		}
		if legacy {
			merged[name] = true
		}
		t, err := g.generated(cmd, name, row)
		if err != nil {
			return nil, err
		}
		if err := add(name, cmd.Path, t); err != nil {
			if skipPlugin(o, cmd, err.Error()) {
				continue
			}
			return nil, err
		}
	}
	for i := range table.Rows {
		row := &table.Rows[i]
		switch {
		case !o.V15 && !row.Shortcut:
			continue // v1.4: the hand tools answer the legacy names
		case row.Kind == KindMerged && merged[row.Name]:
			continue // already the generated tool of its command
		}
		t, err := g.legacyTool(row)
		if err != nil {
			return nil, err
		}
		if err := add(row.Name, row.Target, t); err != nil {
			return nil, err
		}
	}
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]server.ServerTool, 0, len(names))
	for _, n := range names {
		out = append(out, byName[n])
	}
	return out, nil
}

// skipPlugin drops a plugin command's tool with a warning: one installed
// plugin must not stop the server. It reports false for core commands.
func skipPlugin(o Options, cmd *cmdregistry.Command, why string) bool {
	if cmd.Canon != canon.CanonPlugin {
		return false
	}
	if o.Warn != nil {
		o.Warn(fmt.Sprintf("skipping the tool of plugin command %q: %s", cmd.Path, why))
	}
	return true
}

func barePath(p string) string { return strings.TrimPrefix(strings.TrimSpace(p), "nself ") }

func toolName(path string) string { return cmdregistry.ToolName(path) }

// generated builds the tool of one exposed command; row, when set, is the
// merged legacy row of the same name.
func (g *gen) generated(cmd *cmdregistry.Command, name string, row *Row) (server.ServerTool, error) {
	in := invoke.ParamsSchemaIn(g.Registry, cmd)
	if row != nil {
		if err := addLegacyProps(in, row, cmd.Confirm != nil); err != nil {
			return server.ServerTool{}, err
		}
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return server.ServerTool{}, err
	}
	t := mcp.NewToolWithRawSchema(name, describe(cmd), raw)
	t.Annotations = annotations("nself "+barePath(cmd.Path), worstSideEffect(cmd))
	t.RawOutputSchema = g.outputSchema(cmd)
	return server.ServerTool{Tool: t, Handler: g.handler(cmd.Path, row)}, nil
}

// legacyTool builds an alias or removed tool from its row.
func (g *gen) legacyTool(row *Row) (server.ServerTool, error) {
	var cmd *cmdregistry.Command
	if row.Target != "" {
		if c, ok := g.Registry.Lookup(row.Target); ok {
			cmd = c
		} else if row.Plugin == "" {
			return server.ServerTool{}, fmt.Errorf("mcpgen: alias %s targets %q, which the registry does not have", row.Name, row.Target)
		}
	}
	in := map[string]any{"$schema": schemaDialect, "type": "object", "properties": map[string]any{}, "additionalProperties": false}
	if err := addLegacyProps(in, row, cmd != nil && cmd.Confirm != nil); err != nil {
		return server.ServerTool{}, err
	}
	if len(row.Required) > 0 {
		in["required"] = row.Required
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return server.ServerTool{}, err
	}
	side := row.SideEffect
	if cmd != nil {
		side = worstSideEffect(cmd)
	}
	desc := g.aliasDescription(row, cmd)
	t := mcp.NewToolWithRawSchema(row.Name, desc, raw)
	t.Annotations = annotations(row.Name, side)
	t.RawOutputSchema = g.outputSchema(cmd)
	return server.ServerTool{Tool: t, Handler: g.handler(row.Target, row)}, nil
}

func (g *gen) aliasDescription(row *Row, cmd *cmdregistry.Command) string {
	if row.Shortcut {
		return sentence(cmd.Summary) + " Same as " + cmdregistry.ToolName(row.Target) + " with a single query string."
	}
	if row.Kind == KindRemoved {
		return DeprecatedPrefix + "removed in v1.5. " + row.Removed.Message + "; use nself " + strings.Join(row.Removed.Use, " then nself ") + "."
	}
	if cmd != nil {
		if ok, _ := invoke.Exposure(cmd, invoke.TransportMCP, nil); !ok {
			return DeprecatedPrefix + "maps to nself " + row.Target + ", which is not exposed on machine surfaces yet, so calls return E421 until it is; removed in " +
				g.table.RemovalAt + ". " + sentence(cmd.Summary)
		}
	}
	summary := row.Summary
	if cmd != nil {
		summary = cmd.Summary
	}
	return DeprecatedPrefix + "use " + cmdregistry.ToolName(row.Target) + "; removed in " + g.table.RemovalAt + ". " + sentence(summary)
}

func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s != "" && !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

// describe is the summary plus the side-effect sentence.
func describe(cmd *cmdregistry.Command) string {
	d := sentence(cmd.Summary) + " " + map[string]string{
		canon.SideEffectRead:        "Read-only.",
		canon.SideEffectWrite:       "Changes local project state.",
		canon.SideEffectRemote:      "Contacts or changes something outside this project.",
		canon.SideEffectDestructive: "Destructive: can delete or overwrite data.",
	}[cmd.SideEffect]
	if cmd.Confirm != nil {
		d += " A call that needs confirmation returns confirmation_required with a confirm value; repeat the identical call with confirm set to run it."
	}
	return strings.TrimSpace(d)
}

// worstSideEffect is the highest class the command can reach through its exposed flags.
func worstSideEffect(cmd *cmdregistry.Command) string {
	best := cmd.SideEffect
	for _, f := range invoke.ExposedFlags(cmd, invoke.TransportMCP) {
		if f.SideEffect != nil && canon.Rank(*f.SideEffect) > canon.Rank(best) {
			best = *f.SideEffect
		}
	}
	return best
}

func annotations(title, side string) mcp.ToolAnnotation {
	switch side {
	case canon.SideEffectRead, canon.SideEffectWrite, canon.SideEffectRemote, canon.SideEffectDestructive:
	default:
		side = canon.SideEffectDestructive // an undeclared side effect is advertised at the worst class
	}
	return mcp.ToolAnnotation{
		Title:           title,
		ReadOnlyHint:    mcp.ToBoolPtr(side == canon.SideEffectRead),
		DestructiveHint: mcp.ToBoolPtr(side == canon.SideEffectDestructive),
		IdempotentHint:  mcp.ToBoolPtr(side == canon.SideEffectRead),
		OpenWorldHint:   mcp.ToBoolPtr(side == canon.SideEffectRemote),
	}
}
