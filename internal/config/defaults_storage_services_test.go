package config

// Purpose: tests that applyDefaultsMinio runs the MinIO credential guard only when MinIO is enabled (D-0277).
//
// Inputs: a *Config with Env and Minio.Enabled set, and empty or default MinIO credentials.
// Outputs: nil or an error naming MINIO_ROOT_USER.
//
// Constraints: no disk or network; credentials are the built-in defaults, never real values.

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// TestApplyDefaultsMinioDisabledProdLoads: a prod project with MinIO off and
// no MinIO credentials must still load, and its defaults are still applied.
func TestApplyDefaultsMinioDisabledProdLoads(t *testing.T) {
	cfg := &Config{Env: "prod"}
	if err := applyDefaultsMinio(cfg); err != nil {
		t.Fatalf("MinIO disabled must not fail the credential guard: %v", err)
	}
	if cfg.Minio.Port != 9000 || cfg.Minio.Region != "us-east-1" || cfg.Minio.S3Bucket != "nself" {
		t.Errorf("defaults not applied: port=%d region=%q bucket=%q", cfg.Minio.Port, cfg.Minio.Region, cfg.Minio.S3Bucket)
	}
}

// TestApplyDefaultsMinioEnabledProdRejectsDefaults: with MinIO on, prod still
// refuses the built-in minioadmin credentials.
func TestApplyDefaultsMinioEnabledProdRejectsDefaults(t *testing.T) {
	// The legacy built-in service enforces credentials without an installed plugin.
	compattest.Set(t, false)
	cfg := &Config{Env: "prod", Minio: MinioConfig{Enabled: true}}
	err := applyDefaultsMinio(cfg)
	if err == nil || !strings.Contains(err.Error(), "MINIO_ROOT_USER") {
		t.Fatalf("want an error naming MINIO_ROOT_USER, got %v", err)
	}
}

// TestApplyDefaultsMinioEnabledDevAllowsDefaults: dev keeps the minioadmin
// convenience credentials.
func TestApplyDefaultsMinioEnabledDevAllowsDefaults(t *testing.T) {
	cfg := &Config{Env: "dev", Minio: MinioConfig{Enabled: true}}
	if err := applyDefaultsMinio(cfg); err != nil {
		t.Fatalf("dev with default credentials must load: %v", err)
	}
}
