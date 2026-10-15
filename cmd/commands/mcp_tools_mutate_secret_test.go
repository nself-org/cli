package commands

// Purpose: D-0737 prevents legacy MCP calls from overwriting secrets without
// confirmation. Inputs: synthetic secrets and isolated env files. Outputs:
// refusals with no writes. Constraints: v1.4 only, no network or real secrets.
// SPORT: CLI-CMD-MCP-001

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestMCPLegacyConfigSetRefusesSecrets(t *testing.T) {
	compattest.Set(t, false)
	dir := t.TempDir()
	restore := chdir(t, dir)
	defer restore()
	const original = "POSTGRES_PASSWORD=original\nPROJECT_NAME=demo\n"
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"POSTGRES_PASSWORD", "JWT_SECRET", "API_KEY", "AUTH_TOKEN", "DATABASE_DSN"} {
		for _, env := range []string{"", "dev"} {
			t.Run(key+"/"+env, func(t *testing.T) {
				req := mcp.CallToolRequest{}
				req.Params.Arguments = map[string]interface{}{
					"key": key, "value": "attacker-known-value", "env": env,
				}
				res, err := mcpConfigSetHandler()(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				if !res.IsError {
					t.Error("legacy secret write succeeded without confirmation")
				}
				body, err := json.Marshal(res)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(body), "E421") || strings.Contains(string(body), "attacker-known-value") {
					t.Errorf("want E421 refusal without the secret value, got %s", body)
				}
				got, err := os.ReadFile(path)
				if err != nil || string(got) != original {
					t.Errorf("env file changed: %q, %v", got, err)
				}
				if _, err := os.Stat(filepath.Join(dir, ".env.dev")); !os.IsNotExist(err) {
					t.Errorf("secret write created an env file: %v", err)
				}
			})
		}
	}
}
