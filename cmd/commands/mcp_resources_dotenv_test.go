package commands

// mcp_resources_dotenv_test.go — D-0444/D-0558 for the MCP resources that read
// env files: the config resource errors without the file's text, and the env
// cascade resource skips an unreadable file without echoing it.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestMCPResourcesDotenvSyntaxErrorDoesNotLeak(t *testing.T) {
	for _, tc := range dotenvLeakCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			dotenvPlantFile(t, tc.content) // positive control
			defer chdir(t, dir)()

			req := mcp.ReadResourceRequest{}
			req.Params.URI = "nself://config"
			_, err := mcpConfigResourceHandler()(context.Background(), req)
			if err == nil {
				t.Fatal("want a syntax error")
			}
			if strings.Contains(err.Error(), dotenvLeakSecret) || !strings.Contains(err.Error(), "invalid dotenv syntax in ") || !strings.Contains(err.Error(), "(line 2)") {
				t.Errorf("config resource error = %q", err.Error())
			}

			req.Params.URI = "nself://env"
			res, err := mcpEnvResourceHandler()(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range res {
				if tc, ok := c.(mcp.TextResourceContents); ok && strings.Contains(tc.Text, dotenvLeakSecret) {
					t.Errorf("env resource leaks the file text: %q", tc.Text)
				}
			}
		})
	}
}
