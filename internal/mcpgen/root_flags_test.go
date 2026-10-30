package mcpgen_test

// Purpose: a plugin root command that declares flags is a tool with a flags
// member (SHIP-02), and a root that declares none still takes free-form argv.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/mcpgen"
	"github.com/nself-org/cli/schemas"
	"github.com/spf13/cobra"
)

const rootFlagsJSON = `[{"name":"detach","type":"bool","usage":"start the run and return its id"},{"name":"timeout","type":"int","default":"30","usage":"seconds"}]`

func rootFlagTools(t *testing.T, root *cobra.Command, inv mcpgen.Invoker) map[string]mcpTool {
	t.Helper()
	reg, err := advRegistry(t, root)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	list, err := mcpgen.Tools(mcpgen.Options{Registry: reg, Invoke: inv, Schema: schemas.Lookup, V15: true})
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	out := map[string]mcpTool{}
	for _, st := range list {
		out[st.Tool.Name] = mcpTool{name: st.Tool.Name, input: st.Tool.RawInputSchema}
	}
	return out
}

func TestPluginRootFlagsAreToolParams(t *testing.T) {
	root := advNode("ciroot", "run ci", "write", rootFlagsJSON, advNode("history", "past runs", "read", ""))
	tools := rootFlagTools(t, root, noInvoke)
	tool, ok := tools["nself_ciroot"]
	if !ok {
		t.Fatalf("no nself_ciroot tool in %s", nameSet(tools))
	}
	var s struct {
		Properties map[string]struct {
			Properties map[string]struct {
				Type    string `json:"type"`
				Default any    `json:"default"`
			} `json:"properties"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(tool.input, &s); err != nil {
		t.Fatal(err)
	}
	if _, has := s.Properties["argv"]; has {
		t.Errorf("a root with flags must not offer argv: %s", tool.input)
	}
	flags := s.Properties["flags"].Properties
	if flags["detach"].Type != "boolean" || flags["timeout"].Type != "integer" {
		t.Errorf("root flags missing from the tool schema: %s", tool.input)
	}
}

func TestPluginRootFlagsCallReachesInvoker(t *testing.T) {
	root := advNode("ciroot", "run ci", "write", rootFlagsJSON, advNode("history", "past runs", "read", ""))
	reg, err := advRegistry(t, root)
	if err != nil {
		t.Fatal(err)
	}
	var gotPath string
	var got invoke.Request
	inv := func(_ context.Context, path string, req invoke.Request) (invoke.Result, error) {
		gotPath, got = path, req
		return invoke.Result{Stdout: []byte(`{"ok":true}`)}, nil
	}
	list, err := mcpgen.Tools(mcpgen.Options{Registry: reg, Invoke: inv, Schema: schemas.Lookup, V15: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range list {
		if st.Tool.Name != "nself_ciroot" {
			continue
		}
		var req mcp.CallToolRequest
		req.Params.Name = st.Tool.Name
		req.Params.Arguments = map[string]any{"flags": map[string]any{"detach": true}}
		if _, err := st.Handler(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		if gotPath != "ciroot" || got.Flags["detach"] != true || len(got.Argv) != 0 {
			t.Errorf("invoker saw path %q request %+v", gotPath, got)
		}
		return
	}
	t.Fatal("no nself_ciroot tool")
}

func TestPluginRootWithoutFlagsStaysFreeForm(t *testing.T) {
	root := advNode("plainroot", "plain", "write", "")
	tool, ok := rootFlagTools(t, root, noInvoke)["nself_plainroot"]
	if !ok {
		t.Fatal("no nself_plainroot tool")
	}
	var s struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(tool.input, &s); err != nil {
		t.Fatal(err)
	}
	if s.Properties["argv"] == nil || s.Properties["flags"] != nil {
		t.Errorf("a root with no flags keeps argv only: %s", tool.input)
	}
}
