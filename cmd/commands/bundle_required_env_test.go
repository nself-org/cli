package commands

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle/appready"
)

// TestBundleRequiredEnv checks the actual Compose input, including empty
// overrides; a shell-only key cannot satisfy a container's requirement.
func TestBundleRequiredEnv(t *testing.T) {
	for _, tc := range []struct {
		name, env, computed string
		wantMissing         bool
	}{
		{"missing", "PROJECT_NAME=test\n", "", true},
		{"project value", "PROJECT_NAME=test\nTOKENS_ENCRYPTION_KEY=fixture-value\n", "", false},
		{"computed value", "PROJECT_NAME=test\n", "TOKENS_ENCRYPTION_KEY=fixture-value\n", false},
		{"empty override", "PROJECT_NAME=test\nTOKENS_ENCRYPTION_KEY=fixture-value\n", "TOKENS_ENCRYPTION_KEY=\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Chdir(root)
			t.Setenv("TOKENS_ENCRYPTION_KEY", "shell-value-must-not-count")
			write := func(path, data string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			write(filepath.Join(root, ".env"), tc.env)
			if tc.computed != "" {
				write(filepath.Join(root, ".nself", "compose.env"), tc.computed)
			}
			manifest := map[string]any{
				"name": "tokens", "version": "1.0.0", "description": "fixture",
				"envVars": []map[string]any{
					{"name": "TOKENS_ENCRYPTION_KEY", "required": true},
					{"name": "OPTIONAL_INPUT", "required": false},
					{"name": "DEFAULT_INPUT", "required": true, "default": "fallback"},
				},
			}
			encoded, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			pluginDir := filepath.Join(root, "plugins")
			write(filepath.Join(pluginDir, "tokens", "plugin.json"), string(encoded))
			err = appready.RequiredEnvPreflight(bundleComposeEnv)(context.Background(), pluginDir, []string{"tokens"})
			if !tc.wantMissing {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var step *appready.StepError
			if !errors.As(err, &step) || step.Plugin != "tokens" || !strings.Contains(err.Error(), "TOKENS_ENCRYPTION_KEY") {
				t.Fatalf("missing named preflight error: %v", err)
			}
			if strings.Contains(err.Error(), "shell-value") || strings.Contains(err.Error(), "fixture-value") {
				t.Fatal("preflight exposed an input value")
			}
		})
	}
}
