package config

// Purpose: preserve credential enforcement only for MinIO that can run (D-0685).
// Inputs: compat mode, environment, credentials, and installed plugin manifest.
// Outputs: load-time errors for legacy/plugin MinIO; defaults for missing v1.5 plugin.
// Constraints: filesystem fixtures are isolated from the operator's plugins.
// SPORT: D-0685.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestMinioDefaultsCredentialOrder(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		for _, env := range []string{"prod", "staging"} {
			for _, credentials := range []string{"missing", "weak"} {
				for _, manifest := range []string{"absent", "installed", "directory"} {
					for _, pluginPath := range []string{"override", "home"} {
						t.Run(env+"/"+credentials+"/"+manifest+"/"+pluginPath, func(t *testing.T) {
							dir := minioPluginFixture(t, manifest, pluginPath)
							t.Setenv("NSELF_PLUGIN_DIR", dir)
							cfg := &Config{Env: env, Minio: MinioConfig{Enabled: true}}
							if credentials == "weak" {
								cfg.Minio.RootUser = "operator"
								cfg.Minio.RootPassword = "short"
							}
							err := applyDefaultsMinio(cfg)
							wantErr := !compat.V15() || manifest == "installed"
							if (err != nil) != wantErr {
								t.Fatalf("applyDefaultsMinio error = %v, want error=%v", err, wantErr)
							}
							if !wantErr && (cfg.Minio.Port != 9000 || cfg.Minio.S3Bucket != "nself" || !cfg.Minio.Enabled) {
								t.Fatalf("missing-plugin defaults or MinIO intent lost: %+v", cfg.Minio)
							}
						})
					}
				}
			}
		}
	})
}

func minioPluginFixture(t *testing.T, manifest, pluginPath string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".nself", "plugins")
	if pluginPath == "override" {
		dir = t.TempDir()
	}
	path := filepath.Join(dir, "minio", "plugin.json")
	if manifest != "absent" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if manifest == "directory" {
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if pluginPath == "home" {
		return ""
	}
	return dir
}
