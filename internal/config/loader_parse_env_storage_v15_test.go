package config

import (
	"slices"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestObjectStorageMinioIntent(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		for _, enabled := range []string{"true", "1", "YES", "on", "enabled", "false", ""} {
			t.Run("object-storage-"+enabled, func(t *testing.T) {
				clearMinioEnv(t)
				t.Setenv("OBJECT_STORAGE_ENABLED", enabled)
				t.Setenv("S3_ACCESS_KEY", "test-access-key")
				t.Setenv("S3_SECRET_KEY", "fake-secret-not-real-0000")
				cfg := &Config{}
				parseEnvStorage(cfg)
				want := !(compat.V15() && getEnvBool("OBJECT_STORAGE_ENABLED", false))
				if cfg.Minio.Enabled != want {
					t.Fatalf("Minio.Enabled = %v, want %v", cfg.Minio.Enabled, want)
				}
				if cfg.Minio.S3AccessKey != "test-access-key" || cfg.Minio.S3SecretKey != "fake-secret-not-real-0000" {
					t.Fatal("S3 identity fields were lost")
				}
			})
		}
		for _, key := range []string{"MINIO_ROOT_USER", "MINIO_ROOT_PASSWORD", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY", "MINIO_ENABLED", "STORAGE_ENABLED"} {
			t.Run(key, func(t *testing.T) {
				clearMinioEnv(t)
				t.Setenv("OBJECT_STORAGE_ENABLED", "true")
				t.Setenv(key, "true")
				cfg := &Config{}
				parseEnvStorage(cfg)
				if !cfg.Minio.Enabled {
					t.Fatalf("%s no longer enables MinIO", key)
				}
			})
		}
		t.Run("explicit-false", func(t *testing.T) {
			clearMinioEnv(t)
			t.Setenv("OBJECT_STORAGE_ENABLED", "true")
			t.Setenv("MINIO_ENABLED", "false")
			t.Setenv("MINIO_ROOT_USER", "test-access-key")
			cfg := &Config{}
			parseEnvStorage(cfg)
			if cfg.Minio.Enabled {
				t.Fatal("explicit MINIO_ENABLED=false must suppress inference")
			}
		})
	})
}

func TestObjectStorageKnownEnvVars(t *testing.T) {
	for _, key := range []string{"OBJECT_STORAGE_ENABLED", "OBJECT_STORAGE_BUCKETS", "SEAWEEDFS_PORT", "SEAWEEDFS_VERSION"} {
		if !slices.Contains(knownEnvVarsStorage, key) {
			t.Errorf("%s is not a known storage environment variable", key)
		}
	}
}
