package commands

// mcp_tools_ops_dotenv_test.go — D-0444/D-0558 for the MCP config tools: the
// text a tool hands back to a model must not hold the malformed file's text.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func dotenvToolText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestMCPConfigToolsDotenvSyntaxErrorDoesNotLeak(t *testing.T) {
	for _, tc := range dotenvLeakCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			dotenvPlantFile(t, tc.content) // positive control
			defer chdir(t, dir)()

			get := mcp.CallToolRequest{}
			get.Params.Arguments = map[string]interface{}{"key": "GOOD"}
			show := mcp.CallToolRequest{}
			show.Params.Arguments = map[string]interface{}{}
			for name, run := range map[string]func() (*mcp.CallToolResult, error){
				"get":  func() (*mcp.CallToolResult, error) { return mcpConfigGetHandler()(context.Background(), get) },
				"show": func() (*mcp.CallToolResult, error) { return mcpConfigShowHandler()(context.Background(), show) },
			} {
				res, err := run()
				if err != nil || res == nil {
					t.Fatalf("%s: %v", name, err)
				}
				text := dotenvToolText(t, res)
				if strings.Contains(text, dotenvLeakSecret) {
					t.Errorf("%s leaks the file text: %q", name, text)
				}
				if !strings.Contains(text, "invalid dotenv syntax in ") || !strings.Contains(text, "(line 2)") {
					t.Errorf("%s: want a syntax error naming line 2, got %q", name, text)
				}
			}
		})
	}
}
