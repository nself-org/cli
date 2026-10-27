package config

// Purpose: D-0686: S3_SECRET_KEY is validated when OBJECT_STORAGE_ENABLED is on
//          in v1.5+. Empty or short keys fail; strong keys pass; v1.4 ignores it.
// Inputs:  a config with strong Postgres password and Hasura secret.
// Outputs: validatePasswords results for empty, short and strong S3 keys.
// Constraints: staging/prod rules (checkPatterns=true); no disk or network.

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestValidatePasswordsS3SecretKey(t *testing.T) {
	base := func() *Config {
		cfg := &Config{}
		cfg.Postgres.Password = "Xk9vQ2mZ7rT4wB8nL5cD"
		cfg.Hasura.AdminSecret = "Hj3pR8sV1xN6yK0qW5eU9tM2bF7gA4dC"
		return cfg
	}
	for _, tc := range []struct {
		name    string
		v15     bool
		s3Key   string
		wantErr bool
	}{
		{"v1.5 empty key fails", true, "", true},
		{"v1.5 short key fails", true, "short", true},
		{"v1.5 strong key passes", true, "Qm4xT9vB2kR7nZ5wL8cH", false},
		{"v1.4 empty key ignored", false, "", false},
		{"v1.4 short key ignored", false, "short", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			compattest.Set(t, tc.v15)
			t.Setenv("OBJECT_STORAGE_ENABLED", "true")
			cfg := base()
			cfg.Minio.S3SecretKey = tc.s3Key
			err := validatePasswords(cfg, true)
			if tc.wantErr != (err != nil) {
				t.Fatalf("validatePasswords = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "S3_SECRET_KEY") {
				t.Fatalf("error %q does not name S3_SECRET_KEY", err)
			}
		})
	}
}
