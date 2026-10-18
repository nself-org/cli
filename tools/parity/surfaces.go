// Purpose:     count, per top-level command, the MCP tools and HTTP routes the
//
//	generators serve for it (P7-SURF-23).
//
// Inputs:      the machine registry of the prepared v1.5 tree and the tool list
//
//	mcpgen.Tools builds from it. Nothing is text-scanned: a command has an
//	MCP tool exactly when the generator lists the tool name the registry
//	derives for it, and an HTTP route exactly when internal/httpapi's
//	OpenAPI rule (invoke.Exposure for the HTTP transport) exposes it.
//
// Outputs:     surfaceCounts per top-level command name. A count covers the
//
//	command and every command below it (`nself db seed` counts for `db`).
//
// Constraints: legacy alias names are not counted: they are extra names for a
//
//	command that already has its own tool. Installed plugins are out of
//	reach (prepareV15 points the plugin dir at an empty temp dir), so the
//	matrix describes the core surface only and is the same on every host.
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/mcpgen"
	"github.com/nself-org/cli/schemas"
)

// surfaceCounts is the machine surface of one top-level command.
type surfaceCounts struct {
	MCPTools   int
	HTTPRoutes int
}

// generatedToolNames lists the tool names mcpgen serves for reg in v1.5 mode.
func generatedToolNames(reg *cmdregistry.Registry) (map[string]bool, error) {
	stub := func(context.Context, string, invoke.Request) (invoke.Result, error) {
		return invoke.Result{}, fmt.Errorf("parity never runs a command")
	}
	tools, err := mcpgen.Tools(mcpgen.Options{Registry: reg, Invoke: stub, Schema: schemas.Lookup, V15: true})
	if err != nil {
		return nil, fmt.Errorf("generate MCP tools: %w", err)
	}
	names := make(map[string]bool, len(tools))
	for _, t := range tools {
		names[t.Tool.Name] = true
	}
	return names, nil
}

// countSurfaces folds every command of reg into its top-level ancestor.
func countSurfaces(reg *cmdregistry.Registry, tools map[string]bool) map[string]surfaceCounts {
	out := map[string]surfaceCounts{}
	for i := range reg.Commands {
		cmd := &reg.Commands[i]
		top, _, _ := strings.Cut(strings.TrimPrefix(cmd.Path, "nself "), " ")
		c := out[top]
		if ok, _ := invoke.Exposure(cmd, invoke.TransportMCP, nil); ok && tools[cmdregistry.ToolName(cmd.Path)] {
			c.MCPTools++
		}
		if ok, _ := invoke.Exposure(cmd, invoke.TransportHTTP, nil); ok {
			c.HTTPRoutes++
		}
		out[top] = c
	}
	return out
}
