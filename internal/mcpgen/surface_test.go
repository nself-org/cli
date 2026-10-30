package mcpgen

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// TestVisibleWithoutProject: without a project only init and the read-only
// tools are listed; with one, every tool.
func TestVisibleWithoutProject(t *testing.T) {
	tool := func(name string, ro *bool) server.ServerTool {
		t := mcp.NewTool(name)
		t.Annotations.ReadOnlyHint = ro
		return server.ServerTool{Tool: t}
	}
	all := []server.ServerTool{
		tool("nself_init", mcp.ToBoolPtr(false)), tool("nself_status", mcp.ToBoolPtr(true)),
		tool("nself_build", mcp.ToBoolPtr(false)), tool("nself_unannotated", nil),
	}
	var names []string
	for _, x := range visible(all, "") {
		names = append(names, x.Tool.Name)
	}
	if len(names) != 2 || names[0] != "nself_init" || names[1] != "nself_status" {
		t.Fatalf("without a project: %v", names)
	}
	if got := visible(all, "/p"); len(got) != len(all) {
		t.Fatalf("with a project %d of %d tools", len(got), len(all))
	}
}
