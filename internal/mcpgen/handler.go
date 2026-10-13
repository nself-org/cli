package mcpgen

// Purpose: the call path of every generated tool: arguments to a machine
// request, the invoker, the envelope to a tool result.
// Inputs: an MCP tool call. Outputs: a tool result; a Go error is never returned.
// Constraints: a refusal is itself an error envelope (E420 ...), so a client
// always sees the same result shape. The legacy shape of a merged tool, and
// every alias call, adds meta.deprecations.

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/output"
)

func (g *gen) handler(path string, row *Row) server.ToolHandlerFunc {
	command := barePath(path)
	return func(ctx context.Context, call mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return g.call(ctx, command, row, call), nil
	}
}

// rawArguments returns the call's arguments object as sent.
func rawArguments(call mcp.CallToolRequest) ([]byte, error) {
	switch a := call.GetRawArguments().(type) {
	case nil:
		return nil, nil
	case json.RawMessage:
		return a, nil
	default:
		return json.Marshal(a)
	}
}

func (g *gen) call(ctx context.Context, command string, row *Row, call mcp.CallToolRequest) *mcp.CallToolResult {
	name := call.Params.Name
	raw, err := rawArguments(call)
	if err != nil {
		return ErrorResult(command, badParams("%s: the arguments are not JSON", name), nil)
	}
	req, deps, err := g.request(command, row, raw)
	if err != nil {
		return ErrorResult(command, err, deps)
	}
	if row != nil && row.Kind == KindRemoved {
		return ErrorResult(row.Name, removedError(row), deps)
	}
	if row != nil && row.Kind != KindMerged {
		if _, ok := g.Registry.Lookup(command); !ok {
			return ErrorResult(command, errs.Newf("E404", "%s needs the %s plugin, which is not installed", row.Name, row.Plugin).
				WithFix("Run: nself add "+row.Plugin), deps)
		}
	}
	res, err := g.Invoke(ctx, command, req)
	if err != nil {
		return ErrorResult(command, errs.New("E422", "the command could not be run").WithWhy("the invoker failed before the command produced a result"), deps)
	}
	return Result(res.Stdout, deps)
}

// request builds the machine request for a call, and the deprecation notes the
// legacy shape earns.
func (g *gen) request(command string, row *Row, raw []byte) (invoke.Request, []output.Deprecation, error) {
	if row == nil || (row.Kind == KindMerged && len(row.Params) == 0) {
		req, err := invoke.DecodeRequest(raw)
		return req, nil, err
	}
	members := map[string]json.RawMessage{}
	if t := bytes.TrimSpace(raw); len(t) > 0 {
		if t[0] != '{' || json.Unmarshal(t, &members) != nil {
			return invoke.Request{}, nil, badParams("%s: the arguments must be a JSON object", row.Name)
		}
	}
	if row.Kind == KindRemoved {
		return invoke.Request{}, g.deprecations(row, nil), nil
	}
	if row.Kind == KindMerged && !row.IsLegacyShape(members) {
		req, err := invoke.DecodeRequest(raw)
		return req, nil, err
	}
	if err := row.CheckNoMix(members); err != nil {
		return invoke.Request{}, nil, err
	}
	allowConfirm := false
	if cmd, ok := g.Registry.Lookup(command); ok {
		allowConfirm = cmd.Confirm != nil
	}
	req, err := row.Request(members, allowConfirm)
	return req, g.deprecations(row, members), err
}

func (g *gen) deprecations(row *Row, members map[string]json.RawMessage) []output.Deprecation {
	if row.Kind == KindMerged {
		used := joinNames(row.legacyPresent(members))
		return []output.Deprecation{{Old: row.Name + " {" + used + "}", New: row.Name + " {args, flags}", RemovalAt: g.table.RemovalAt}}
	}
	if row.Kind == KindRemoved {
		return []output.Deprecation{{Old: row.Name, New: "nself " + row.Removed.Use[0], RemovalAt: g.table.RemovalAt}}
	}
	return []output.Deprecation{{Old: row.Name, New: cmdNameOf(row), RemovalAt: g.table.RemovalAt}}
}

func cmdNameOf(row *Row) string { return toolName(row.Target) }

func removedError(row *Row) error {
	use := "Use nself " + row.Removed.Use[0]
	for _, u := range row.Removed.Use[1:] {
		use += " and then nself " + u
	}
	return errs.New(row.Removed.Code, row.Removed.Message).WithFix(use + ".")
}
