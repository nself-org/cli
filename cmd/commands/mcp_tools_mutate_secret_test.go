package commands

// Purpose: proves the v1.4 legacy nself_config_set tool refuses secret keys
// (D-0737) and still writes ordinary keys; in v1.5 the generated gated tool
// replaces it, so the legacy guard does not apply there.
// Inputs: synthetic keys and a temp project. Outputs: pass/fail.
// Constraints: no network, temp dirs only.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestMCPConfigSetRefusesSecretKeyInLegacyMode(t *testing.T) {
	compattest.Set(t, false)
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("PROJECT_NAME=demo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	defer chdir(t, dir)()

	for _, key := range []string{"POSTGRES_PASSWORD", "HASURA_GRAPHQL_ADMIN_SECRET", "S3_SECRET_KEY", "BASIC_AUTH"} {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{"key": key, "value": "synthetic-value"}
		res, err := mcpConfigSetHandler()(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		text := res.Content[0].(mcp.TextContent).Text
		if !strings.Contains(text, "requires the v1.5 gated tool") {
			t.Errorf("%s: want a secret-key refusal, got %q", key, text)
		}
	}
	got, _ := os.ReadFile(envPath)
	if string(got) != "PROJECT_NAME=demo\n" {
		t.Errorf(".env changed by a refused call: %q", got)
	}

	// A non-secret key still passes through.
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"key": "ABC", "value": "v"}
	if res, err := mcpConfigSetHandler()(context.Background(), req); err != nil || !isConfigSetOK(res) {
		t.Fatalf("non-secret key refused: %v %#v", err, res)
	}
}

func TestMCPConfigSetSecretGuardIsLegacyOnly(t *testing.T) {
	compattest.Set(t, true)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=demo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	defer chdir(t, dir)()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"key": "POSTGRES_PASSWORD", "value": "synthetic-value"}
	res, err := mcpConfigSetHandler()(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !isConfigSetOK(res) {
		t.Errorf("v1.5 hit the legacy guard: %#v", res)
	}
}

// isConfigSetOK reports whether the handler took the success path.
func isConfigSetOK(res *mcp.CallToolResult) bool {
	_, ok := res.StructuredContent.(ConfigSetResult)
	return ok
}
