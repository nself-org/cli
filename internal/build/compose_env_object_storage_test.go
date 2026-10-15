package build

// Purpose: prove plugin compose fragments receive the active object store connection.
// Inputs: v1.4/v1.5 configs and a synthetic installed MinIO plugin.
// Outputs: assertions against the actual generated .nself/compose.env file.
// Constraints: tests select compatibility mode explicitly and never need Docker.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
)

func TestComposeEnvCarriesActiveObjectStore(t *testing.T) {
	tests := []struct {
		name      string
		v15       bool
		seaweed   bool
		enabled   bool
		installed bool
		port      int
		region    string
		want      map[string]string
	}{
		{
			name: "v1.4 core MinIO honors endpoint and region overrides", enabled: true,
			port: 19000, region: "eu-west-2",
			want: map[string]string{"S3_ENDPOINT": "http://minio:19000", "S3_REGION": "eu-west-2"},
		},
		{
			name: "v1.5 core SeaweedFS wins over installed MinIO", v15: true, seaweed: true, enabled: true, installed: true,
			region: "ap-southeast-1",
			want:   map[string]string{"S3_ENDPOINT": "http://seaweedfs:8333", "S3_REGION": "ap-southeast-1"},
		},
		{
			name: "v1.5 MinIO plugin uses its internal endpoint", v15: true, enabled: true, installed: true,
			port: 19000, region: "ca-central-1",
			want: map[string]string{"S3_ENDPOINT": "http://minio:9000", "S3_REGION": "ca-central-1"},
		},
		{
			name: "disabled store emits no plugin fallback", v15: true, installed: true,
			region: "us-east-2", want: map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			compattest.Set(t, tc.v15)
			t.Setenv("OBJECT_STORAGE_ENABLED", "false")
			if tc.seaweed {
				t.Setenv("OBJECT_STORAGE_ENABLED", "true")
			}
			workdir, pluginDir := t.TempDir(), t.TempDir()
			if err := os.WriteFile(filepath.Join(workdir, ".env"), []byte("S3_ENDPOINT=http://external:9443\nS3_REGION=external-1\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if tc.installed {
				dir := filepath.Join(pluginDir, "minio")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"name":"minio"}`), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			cfg := &config.Config{ProjectName: "store-test"}
			cfg.Minio.Enabled = tc.enabled
			cfg.Minio.Port = tc.port
			cfg.Minio.Region = tc.region
			vars := ComputePluginEnvVars(workdir, pluginDir, cfg)
			if err := WriteComposeEnv(workdir, cfg, nil, vars); err != nil {
				t.Fatalf("WriteComposeEnv: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(workdir, composeEnvFile))
			if err != nil {
				t.Fatal(err)
			}
			got := string(data)
			for key, value := range tc.want {
				if !strings.Contains(got, key+"="+value+"\n") {
					t.Errorf("compose.env missing %s=%s:\n%s", key, value, got)
				}
			}
			if len(tc.want) == 0 && (strings.Contains(got, "S3_ENDPOINT=") || strings.Contains(got, "S3_REGION=")) {
				t.Errorf("disabled store wrote S3 connection vars:\n%s", got)
			}
			resolved := map[string]string{}
			for _, path := range ComposeEnvFiles(workdir) {
				values, readErr := config.ReadEnvFile(path)
				if readErr != nil {
					t.Fatal(readErr)
				}
				for key, value := range values {
					resolved[key] = value
				}
			}
			if len(tc.want) == 0 {
				if resolved["S3_ENDPOINT"] != "http://external:9443" || resolved["S3_REGION"] != "external-1" {
					t.Errorf("no-store fallback lost explicit project values: %v", resolved)
				}
			} else {
				for key, value := range tc.want {
					if resolved[key] != value {
						t.Errorf("resolved %s=%q want computed %q", key, resolved[key], value)
					}
				}
			}
		})
	}
}

func TestComposeEnvOpsProfileOmitsObjectStore(t *testing.T) {
	compattest.Set(t, true)
	t.Setenv("OBJECT_STORAGE_ENABLED", "true")
	workdir, pluginDir := t.TempDir(), t.TempDir()
	cfg := &config.Config{ProjectName: "ops-test"}
	cfg.Minio.Region = "us-east-1"

	vars := computePluginEnvVarsForProfile(workdir, pluginDir, cfg, false)
	if err := WriteComposeEnv(workdir, cfg, nil, vars); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(workdir, composeEnvFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "S3_ENDPOINT=") || strings.Contains(string(data), "S3_REGION=") {
		t.Fatalf("ops profile wrote object store vars:\n%s", data)
	}
}
