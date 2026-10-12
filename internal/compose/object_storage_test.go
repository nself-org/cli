package compose

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
)

func storageFixture(t *testing.T, minio, object, custom bool) *config.Config {
	t.Helper()
	t.Setenv("OBJECT_STORAGE_ENABLED", "false")
	t.Setenv("OBJECT_STORAGE_BUCKETS", "uploads,assets")
	t.Setenv("SEAWEEDFS_PORT", "")
	if object {
		t.Setenv("OBJECT_STORAGE_ENABLED", "true")
	}
	cfg := minimalConfig()
	cfg.ProjectName = "storage-fixture"
	cfg.Minio.Enabled = minio
	cfg.Minio.RootUser = "test-access-key"
	cfg.Minio.RootPassword = "fake-secret-not-real-0000"
	cfg.Minio.S3AccessKey = "test-access-key"
	cfg.Minio.S3SecretKey = "fake-secret-not-real-0000"
	if custom {
		cfg.CustomServices = []config.CustomService{{Name: "worker", Port: 3000}}
	}
	return cfg
}

func storageGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "object_storage", name+".yaml")
	if os.Getenv("UPDATE_OBJECT_STORAGE_GOLDENS") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("compose differs from base golden %s", path)
	}
}

// TestSeaweedV14Unchanged records the base generator before the v1.5 implementation.
func TestSeaweedV14Unchanged(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		for _, tc := range []struct {
			name                  string
			minio, object, custom bool
		}{
			{"minio", true, false, false}, {"object", false, true, false},
			{"none", false, false, false}, {"custom", true, false, true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				cfg := storageFixture(t, tc.minio, tc.object, tc.custom)
				got, err := NewGenerator(cfg).Generate()
				if err != nil {
					t.Fatal(err)
				}
				storageGolden(t, "v14-"+tc.name, got)
			})
		}
	})
}
