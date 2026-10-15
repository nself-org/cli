package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
)

func TestDiscoverMinioProjectIntent(t *testing.T) {
	for _, tc := range []struct {
		name     string
		v15      bool
		env      map[string]string
		disabled bool
		want     bool
	}{
		{"v15 no intent", true, nil, false, false},
		{"v15 explicit false with credentials", true, map[string]string{"MINIO_ENABLED": "false", "MINIO_ROOT_USER": "fixture"}, false, false},
		{"v15 explicit true", true, map[string]string{"MINIO_ENABLED": "true"}, false, true},
		{"v15 legacy storage toggle", true, map[string]string{"STORAGE_ENABLED": "true"}, false, true},
		{"v15 inferred credentials", true, map[string]string{"MINIO_ROOT_USER": "fixture"}, false, true},
		{"v15 core S3 only", true, map[string]string{"OBJECT_STORAGE_ENABLED": "true", "S3_ACCESS_KEY": "fixture"}, false, false},
		{"v15 both stores for migration", true, map[string]string{"OBJECT_STORAGE_ENABLED": "true", "MINIO_ENABLED": "true"}, false, true},
		{"v15 disabled plugin", true, map[string]string{"MINIO_ENABLED": "true"}, true, false},
		{"v14 unchanged", false, map[string]string{"MINIO_ENABLED": "false"}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateEnv(t)
			compattest.Set(t, tc.v15)
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			project, plugins := t.TempDir(), t.TempDir()
			cfg, err := config.Load(project)
			if err != nil {
				t.Fatal(err)
			}
			if tc.v15 && cfg.Minio.Enabled != (tc.want || tc.disabled) {
				t.Fatalf("fixture Minio.Enabled = %v", cfg.Minio.Enabled)
			}
			fragment := "services:\n  fixture:\n    image: alpine:latest\n"
			installPlugin(t, plugins, "minio", `{}`, fragment)
			installPlugin(t, plugins, "other", `{}`, fragment)
			if tc.disabled {
				writeFile(t, filepath.Join(plugins, "minio", ".disabled"), "")
			}
			files, err := DiscoverPluginComposeFiles(project, plugins)
			if err != nil {
				t.Fatal(err)
			}
			wantCount := 1
			if tc.want {
				wantCount++
			}
			if len(files) != wantCount || files[len(files)-1] != filepath.Join(plugins, "other", pluginComposeFilename) {
				t.Fatalf("fragments = %v; want minio=%v and other", files, tc.want)
			}
			if !tc.want {
				data, err := os.ReadFile(filepath.Join(plugins, "minio", pluginComposeFilename))
				if err != nil || string(data) != fragment {
					t.Fatalf("excluded MinIO fragment was changed: %q, %v", data, err)
				}
			}
		})
	}
}
