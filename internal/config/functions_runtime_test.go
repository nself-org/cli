package config_test

// Purpose: exercise FUNCTIONS_RUNTIME through the real .env loader and the
// P7-LIVE-09 ImageInputs matrix, without setting the parsed field manually.
// Inputs: runtime and version values in a project .env.
// Outputs: loader validation and rendered image assertions.
// Constraints: process environment is restored; tests must not run in parallel.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"gopkg.in/yaml.v3"
)

func functionsProject(t *testing.T, runtime, version string, enabled bool) string {
	t.Helper()
	for _, key := range []string{
		"ENV", "PROJECT_NAME", "BASE_DOMAIN", "FUNCTIONS_RUNTIME",
		"FUNCTIONS_ENABLED", "FUNCTIONS_VERSION", "FUNCTIONS_PORT", "FUNCTIONS_ROUTE",
		"FUNCTIONS_IMAGE", "FUNCTIONS_IMAGE_DIGEST", "DENO_IMAGE", "PYTHON_IMAGE",
		"IMAGE_PINNING", "REDIS_ENGINE", "NSELF_V15", "OBJECT_STORAGE_ENABLED",
		"MINIO_ENABLED", "STORAGE_ENABLED", "MINIO_ROOT_USER", "MINIO_ROOT_PASSWORD",
		"MINIO_ACCESS_KEY", "MINIO_SECRET_KEY", "S3_ACCESS_KEY", "S3_SECRET_KEY",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("IMAGE_PINNING", compose.PinningLegacy)
	dir := t.TempDir()
	data := fmt.Sprintf("ENV=dev\nPROJECT_NAME=runtime-test\nBASE_DOMAIN=runtime.test\nFUNCTIONS_ENABLED=%t\nFUNCTIONS_RUNTIME=%s\nFUNCTIONS_VERSION=%s\n", enabled, runtime, version)
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadFunctionsRuntime(t *testing.T) {
	for _, runtime := range []string{"", "node", "deno", "python"} {
		t.Run("runtime="+runtime, func(t *testing.T) {
			cfg, err := config.Load(functionsProject(t, runtime, "", true))
			if err != nil {
				t.Fatal(err)
			}
			want := runtime
			if want == "" {
				want = "node"
			}
			if cfg.Functions.Runtime != want {
				t.Fatalf("Functions.Runtime = %q, want %q from .env", cfg.Functions.Runtime, want)
			}
		})
	}
	for _, runtime := range []string{"ruby", "DENO", "nodejs"} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("invalid=%s/enabled=%t", runtime, enabled), func(t *testing.T) {
				_, err := config.Load(functionsProject(t, runtime, "", enabled))
				if err == nil || !strings.Contains(err.Error(), "FUNCTIONS_RUNTIME") {
					t.Fatalf("Load = %v, want FUNCTIONS_RUNTIME validation error", err)
				}
			})
		}
	}
}

func functionsImage(t *testing.T, runtime, version string) string {
	t.Helper()
	cfg, err := config.Load(functionsProject(t, runtime, version, true))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := compose.NewGenerator(cfg).Generate()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Services map[string]struct {
			Image string `yaml:"image"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	image := doc.Services["functions"].Image
	if image == "" {
		t.Fatal("functions service has no image")
	}
	return image
}

func TestFunctionsRuntimeImageInputsThroughLoader(t *testing.T) {
	for _, tc := range []struct{ runtime, entry string }{
		{"", "functions"}, {"node", "functions"}, {"deno", "deno"}, {"python", "python"},
	} {
		t.Run("runtime="+tc.runtime, func(t *testing.T) {
			base := functionsImage(t, tc.runtime, "")
			if want := compose.ImageRef(tc.entry, ""); base != want {
				t.Fatalf("runtime %q renders %q, want %q", tc.runtime, base, want)
			}
			var row *compose.ImageInput
			for _, in := range compose.ImageInputs() {
				if in.Service == "functions" && in.Entry == tc.entry {
					row = &in
					break
				}
			}
			if row == nil {
				t.Fatalf("ImageInputs has no functions/%s row", tc.entry)
			}
			listed := len(row.Vars) == 1 && row.Vars[0] == "FUNCTIONS_VERSION"
			if tc.entry != "functions" && len(row.Vars) != 0 {
				t.Fatalf("%s runtime must not list version inputs: %v", tc.entry, row.Vars)
			}
			changed := functionsImage(t, tc.runtime, "9.9.9-sentinel") != base
			if changed != listed || listed != (tc.entry == "functions") {
				t.Fatalf("functions/%s: version changes image=%t, ImageInputs lists FUNCTIONS_VERSION=%t", tc.entry, changed, listed)
			}
		})
	}
}
