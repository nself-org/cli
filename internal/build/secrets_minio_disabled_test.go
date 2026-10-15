package build

import (
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func TestSecretEnvMapMinioEnabled(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		cfg := &config.Config{Minio: config.MinioConfig{Enabled: enabled, RootUser: "minioadmin", RootPassword: "minioadmin", S3AccessKey: "access", S3SecretKey: "secret"}}
		secrets := SecretEnvMap(cfg)
		for _, key := range []string{"MINIO_ROOT_USER", "MINIO_ROOT_PASSWORD"} {
			if _, found := secrets[key]; found != enabled {
				t.Errorf("MinIO enabled=%v: %s present=%v", enabled, key, found)
			}
		}
		// SeaweedFS also uses S3 keys while MinIO is disabled.
		if secrets["S3_ACCESS_KEY"] != "access" || secrets["S3_SECRET_KEY"] != "secret" {
			t.Errorf("MinIO enabled=%v: shared S3 credentials omitted", enabled)
		}
	}
}
