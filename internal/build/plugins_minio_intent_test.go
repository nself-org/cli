package build

// plugins_minio_intent_test.go: D-0702. The minio plugin is installed machine-wide,
// so its compose fragment joins a project's stack only where the project wants
// MinIO (v1.5); v1.4 keeps merging every enabled plugin. Another plugin merges
// in every case, and the check reads the manifest a real Build writes.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestBuildMergesMinioFragmentOnlyWithMinioIntent(t *testing.T) {
	tests := []struct {
		name         string
		v15, enabled bool
		wantMinio    bool
	}{
		{"v1.4 intent off", false, false, true},
		{"v1.4 intent on", false, true, true},
		{"v1.5 intent off", true, false, false},
		{"v1.5 intent on", true, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := freshProject(t, "dev-minimal") // isolates the env, so the mode is set after it
			compattest.Set(t, tc.v15)
			t.Setenv("OBJECT_STORAGE_ENABLED", "false")
			writePlanPlugin(t, f.plugins, "minio", 3911, false)
			writePlanPlugin(t, f.plugins, "nself-alpha", 3901, false)
			if tc.enabled {
				env := filepath.Join(f.workdir, ".env")
				data, err := os.ReadFile(env)
				if err != nil {
					t.Fatal(err)
				}
				writeFixtureFile(t, env, string(data)+"\nMINIO_ENABLED=true\nMINIO_ROOT_USER=minio-user-Tq8Zk\nMINIO_ROOT_PASSWORD=Vb3Nc7Rx9Wt5Kq2Hy8Zp4Lm6Jd1Sa0Fe\n", 0o600)
			}
			if _, err := Build(f.workdir, BuildOptions{}); err != nil {
				t.Fatalf("build: %v", err)
			}
			raw, err := os.ReadFile(filepath.Join(f.workdir, composeManifestFile))
			if err != nil {
				t.Fatal(err)
			}
			manifest := filepath.ToSlash(string(raw))
			if got := strings.Contains(manifest, "/minio/"); got != tc.wantMinio {
				t.Errorf("minio fragment in manifest = %v, want %v:\n%s", got, tc.wantMinio, manifest)
			}
			if !strings.Contains(manifest, "/nself-alpha/") {
				t.Errorf("control plugin missing from manifest:\n%s", manifest)
			}
		})
	}
}
