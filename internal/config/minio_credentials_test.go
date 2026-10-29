package config

// Purpose: Unit tests for ValidateMinioCredentials (MINIO-CRED-01).
//
// Inputs: *Config with Env, Minio.RootUser, Minio.RootPassword set.
// Outputs: error (non-nil) or nil.
//
// Constraints:
//   - prod/staging + "minioadmin" credentials → must error with SECURITY prefix.
//   - dev + "minioadmin" credentials → must return nil (dev is unblocked).
//   - prod + strong credentials → must return nil.
//   - prod + password <16 chars → must error.
//   - prod + empty RootUser → must error.
//
// SPORT: F09 — MINIO_ROOT_USER, MINIO_ROOT_PASSWORD annotated as blocked-in-prod.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestValidateMinioCredentialsDisabled(t *testing.T) {
	compattest.Set(t, false)
	for _, env := range []string{"prod", "staging"} {
		cfg := &Config{Env: env, Minio: MinioConfig{RootUser: "minioadmin", RootPassword: "minioadmin"}}
		if err := ValidateMinioCredentials(cfg); err != nil {
			t.Errorf("disabled MinIO in %s: %v", env, err)
		}
		if err := Validate(cfg); err != nil && strings.Contains(err.Error(), "MINIO_") {
			t.Errorf("registered validators rejected disabled MinIO in %s: %v", env, err)
		}
	}
}

func TestValidateMinioCredentials_ProdMinioadmin_ReturnsError(t *testing.T) {
	compattest.Set(t, false)
	cfg := &Config{
		Env: "prod",
		Minio: MinioConfig{
			Enabled:      true,
			RootUser:     "minioadmin",
			RootPassword: "minioadmin",
		},
	}
	err := ValidateMinioCredentials(cfg)
	if err == nil {
		t.Fatal("expected error for prod+minioadmin user, got nil")
	}
	if !strings.Contains(err.Error(), "SECURITY") {
		t.Errorf("expected SECURITY prefix in error, got: %s", err.Error())
	}
}

func TestValidateMinioCredentials_StagingMinioadmin_ReturnsError(t *testing.T) {
	compattest.Set(t, false)
	cfg := &Config{
		Env: "staging",
		Minio: MinioConfig{
			Enabled:      true,
			RootUser:     "minioadmin",
			RootPassword: "minioadmin",
		},
	}
	err := ValidateMinioCredentials(cfg)
	if err == nil {
		t.Fatal("expected error for staging+minioadmin user, got nil")
	}
}

func TestValidateMinioCredentials_DevMinioadmin_ReturnsNil(t *testing.T) {
	compattest.Set(t, false)
	cfg := &Config{
		Env: "dev",
		Minio: MinioConfig{
			Enabled:      true,
			RootUser:     "minioadmin",
			RootPassword: "minioadmin",
		},
	}
	err := ValidateMinioCredentials(cfg)
	if err != nil {
		t.Errorf("expected nil for dev+minioadmin, got: %s", err.Error())
	}
}

func TestValidateMinioCredentials_ProdStrongCredentials_ReturnsNil(t *testing.T) {
	compattest.Set(t, false)
	cfg := &Config{
		Env: "prod",
		Minio: MinioConfig{
			Enabled:      true,
			RootUser:     "prod-minio-user",
			RootPassword: "v3ryStr0ngP@ssw0rd!!",
		},
	}
	err := ValidateMinioCredentials(cfg)
	if err != nil {
		t.Errorf("expected nil for prod with strong credentials, got: %s", err.Error())
	}
}

func TestValidateMinioCredentials_ProdShortPassword_ReturnsError(t *testing.T) {
	compattest.Set(t, false)
	cfg := &Config{
		Env: "prod",
		Minio: MinioConfig{
			Enabled:      true,
			RootUser:     "prod-minio-user",
			RootPassword: "short",
		},
	}
	err := ValidateMinioCredentials(cfg)
	if err == nil {
		t.Fatal("expected error for prod+short password, got nil")
	}
	if !strings.Contains(err.Error(), "SECURITY") {
		t.Errorf("expected SECURITY prefix in error, got: %s", err.Error())
	}
}

func TestValidateMinioCredentials_ProdEmptyUser_ReturnsError(t *testing.T) {
	compattest.Set(t, false)
	cfg := &Config{
		Env: "prod",
		Minio: MinioConfig{
			Enabled:      true,
			RootUser:     "",
			RootPassword: "v3ryStr0ngP@ssw0rd!!",
		},
	}
	err := ValidateMinioCredentials(cfg)
	if err == nil {
		t.Fatal("expected error for prod+empty user, got nil")
	}
}

func TestValidateMinioCredentials_ProdMinioadminPassword_StrongUser_ReturnsError(t *testing.T) {
	compattest.Set(t, false)
	cfg := &Config{
		Env: "prod",
		Minio: MinioConfig{
			Enabled:      true,
			RootUser:     "prod-minio-user",
			RootPassword: "minioadmin",
		},
	}
	err := ValidateMinioCredentials(cfg)
	if err == nil {
		t.Fatal("expected error for prod+minioadmin password with strong user, got nil")
	}
	if !strings.Contains(err.Error(), "MINIO_ROOT_PASSWORD") {
		t.Errorf("expected MINIO_ROOT_PASSWORD in error, got: %s", err.Error())
	}
}

// D-0685: v1.5 skips the credential guard while the minio plugin is missing (E138 refuses
// the build with the real remedy); plugin-backed MinIO and v1.4 keep the guard.
func TestValidateMinioCredentials_PluginGate(t *testing.T) {
	weak := func(env string) *Config {
		return &Config{Env: env, Minio: MinioConfig{Enabled: true, RootUser: "minioadmin", RootPassword: "minioadmin"}}
	}
	for _, tc := range []struct {
		name      string
		v15       bool
		installed bool
		wantErr   bool
	}{
		{"v14-no-plugin", false, false, true},
		{"v14-plugin", false, true, true},
		{"v15-no-plugin", true, false, false},
		{"v15-plugin", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			compattest.Set(t, tc.v15)
			dir := t.TempDir()
			t.Setenv("NSELF_PLUGIN_DIR", dir)
			if tc.installed {
				if err := os.MkdirAll(filepath.Join(dir, "minio"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "minio", "plugin.json"), []byte("{}"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for _, env := range []string{"prod", "staging"} {
				err := ValidateMinioCredentials(weak(env))
				if (err != nil) != tc.wantErr {
					t.Errorf("%s: err=%v, wantErr=%v", env, err, tc.wantErr)
				}
			}
		})
	}
}
