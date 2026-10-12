package commands

// Purpose: proves the legacy MCP config tools refuse an env argument that
// would leave the project directory (D-0738). Inputs are synthetic env names
// and a temp project; outputs are pass/fail. Constraints: no network, temp
// dirs only; the outside file lives next to the project, never in a real path.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/nself-org/cli/internal/errs"
)

func TestValidateMCPEnvName(t *testing.T) {
	cases := []struct {
		name string
		env  string
		ok   bool
	}{
		{"empty is plain .env", "", true},
		{"dev", "dev", true},
		{"staging", "staging", true},
		{"hyphen and digit", "prod-2", true},
		{"leading digit", "1a", true},
		{"underscore", "qa_east", true},
		{"63 characters", strings.Repeat("a", 63), true},
		{"64 characters", strings.Repeat("a", 64), false},
		{"trailing newline", "dev\n", false},
		{"traversal", "/../../../tmp/x", false},
		{"relative traversal", "../x", false},
		{"dot dot", "..", false},
		{"absolute path", "/etc/passwd", false},
		{"forward slash", "a/b", false},
		{"backslash", `a\b`, false},
		{"uppercase", "Prod", false},
		{"leading underscore", "_dev", false},
		{"leading hyphen", "-dev", false},
		{"dot", "dev.bak", false},
		{"space", "dev ", false},
		{"newline", "dev\nx", false},
		{"null byte", "dev\x00", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMCPEnvName(tc.env)
			if tc.ok {
				if err != nil {
					t.Fatalf("validateMCPEnvName(%q) = %v, want nil", tc.env, err)
				}
				return
			}
			var ce *errs.CLIError
			if !errors.As(err, &ce) {
				t.Fatalf("validateMCPEnvName(%q) = %v, want a *errs.CLIError", tc.env, err)
			}
			if ce.Code != "E401" || ce.Why == "" || ce.Fix == "" {
				t.Errorf("error = %+v, want code E401 with why and fix", ce)
			}
		})
	}
}

// TestMCPConfigToolsRefuseEnvTraversal drives the three legacy handlers with
// an env that climbs out of the project and checks that nothing outside is
// written or read.
func TestMCPConfigToolsRefuseEnvTraversal(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".env"), []byte("PROJECT_NAME=demo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// ".env.<env>" with env "/../outside" resolves to <root>/outside.
	outside := filepath.Join(root, "outside")
	if err := os.WriteFile(outside, []byte("SYNTHETIC_MARKER=readable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	restore := chdir(t, project)
	defer restore()

	bad := "/../outside"
	call := func(h func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]interface{}) string {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = args
		res, err := h(context.Background(), req)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if res.StructuredContent != nil {
			t.Fatalf("expected an error result, got structured content %#v", res.StructuredContent)
		}
		return res.Content[0].(mcp.TextContent).Text
	}

	for name, out := range map[string]string{
		"set":  call(mcpConfigSetHandler(), map[string]interface{}{"key": "ABC", "value": "v", "env": bad}),
		"get":  call(mcpConfigGetHandler(), map[string]interface{}{"key": "SYNTHETIC_MARKER", "env": bad}),
		"show": call(mcpConfigShowHandler(), map[string]interface{}{"env": bad}),
	} {
		if !strings.Contains(out, "E401") || strings.Contains(out, "readable") {
			t.Errorf("%s: output = %q, want an E401 refusal", name, out)
		}
	}
	got, err := os.ReadFile(outside)
	if err != nil || string(got) != "SYNTHETIC_MARKER=readable\n" {
		t.Errorf("outside file changed: %q, %v", got, err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 2 {
		t.Errorf("unexpected files created under %s: %v", root, entries)
	}
}

// TestMCPConfigSetAcceptsValidEnv keeps the normal path working.
func TestMCPConfigSetAcceptsValidEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=demo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	restore := chdir(t, dir)
	defer restore()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"key": "ABC", "value": "v", "env": "staging"}
	res, err := mcpConfigSetHandler()(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.StructuredContent.(ConfigSetResult); !ok {
		t.Fatalf("want ConfigSetResult, got %#v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.staging")); err != nil {
		t.Errorf(".env.staging not written: %v", err)
	}
}
