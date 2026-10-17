package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestRemoteDeployProcessOnlyConfig(t *testing.T) {
	compattest.Set(t, true)
	isolateRemoteConfigEnv(t)
	for _, key := range []string{"POSTGRES_PASSWORD", "CLOUDFLARE_API_KEY", "AUTH_PROVIDER_TEST_SECRET", "CS_1_ENV", "FRONTEND_APP_1_IMAGE", "INTERNAL_ROUTE_1_TARGET"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("ENV", "staging")
			t.Setenv(key, "process-secret-sentinel")
			_, err := LoadWithOptions(t.TempDir(), LoadOptions{RemoteDeploy: true})
			if err == nil {
				t.Fatalf("remote build accepted process-only %s", key)
			}
			if !strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), "env file") || strings.Contains(err.Error(), "process-secret-sentinel") {
				t.Fatalf("unsafe or unactionable refusal: %v", err)
			}
		})
	}
}

// v1.4 keeps the old behaviour (warning, no refusal) until the 1.5 default flip.
func TestRemoteDeployProcessOnlyConfigV14Warns(t *testing.T) {
	compattest.Set(t, false)
	isolateRemoteConfigEnv(t)
	t.Setenv("ENV", "staging")
	t.Setenv("POSTGRES_PASSWORD", "process-secret-sentinel")
	if _, err := LoadWithOptions(t.TempDir(), LoadOptions{RemoteDeploy: true}); err != nil {
		t.Fatalf("v1.4 remote build refused process-only config: %v", err)
	}
}

func TestRemoteDeployFilePrecedence(t *testing.T) {
	compattest.Set(t, true)
	isolateRemoteConfigEnv(t)
	t.Setenv("ENV", "staging")
	t.Setenv("POSTGRES_PASSWORD", "shell-secret")
	t.Setenv("UNRELATED_SHELL_SECRET", "shell-only")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("POSTGRES_PASSWORD=file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadWithOptions(dir, LoadOptions{RemoteDeploy: true})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Postgres.Password != "file-secret" {
		t.Fatalf("file password lost precedence")
	}
}

func isolateRemoteConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range knownEnvVars {
		t.Setenv(key, "")
	}
	for key := range remoteExtraConfigKeys {
		t.Setenv(key, "")
	}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if remoteDynamicConfigKey(key) {
			t.Setenv(key, "")
		}
	}
}

func TestRemoteDeployIgnoresLocalOnlyKeys(t *testing.T) {
	compattest.Set(t, true)
	isolateRemoteConfigEnv(t)
	for key := range localOnlyConfigKeys {
		t.Setenv(key, "/local/only")
	}
	if err := ValidateRemoteEnv(map[string]string{}); err != nil {
		t.Fatalf("local-only CLI keys must not block a remote deploy: %v", err)
	}
}
