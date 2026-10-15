package config

// Purpose: regression tests for D-0686 — the SeaweedFS identity key
// S3_SECRET_KEY gets the same staging/prod strength check as the other
// service secrets when object storage is enabled.
//
// Constraints:
//   - staging/prod + storage enabled + short or insecure key → error.
//   - dev, storage disabled, or an unset key (E142 and the root-password
//     fallback own that case) → no error.

import (
	"errors"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func s3SecretCfg(env string, enabled bool, secret string) *Config {
	return &Config{
		Env:      env,
		Postgres: PostgresConfig{Password: "Kq8vZr2LmX9tB4wYnP"},
		Hasura:   HasuraConfig{AdminSecret: "Vf7Jd3Hs9Lw2Qa8Zx5Cn1Bm6Tr4Ye0UgKo"},
		Minio: MinioConfig{
			Enabled:      enabled,
			RootPassword: "Wn4Rb7Tj2Xc9Lf5Hq",
			S3SecretKey:  secret,
		},
	}
}

func TestValidatePasswords_S3SecretKey(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr error
	}{
		{"prod short key", s3SecretCfg("prod", true, "short"), errs.ErrWeakPassword},
		{"staging short key", s3SecretCfg("staging", true, "short"), errs.ErrWeakPassword},
		{"prod insecure pattern", s3SecretCfg("prod", true, "my-password-1234567890"), errs.ErrInsecurePassword},
		{"prod strong key", s3SecretCfg("prod", true, "Pz6Mk3Vd8Gs1Yh7Ub4Nx"), nil},
		{"prod unset key", s3SecretCfg("prod", true, ""), nil},
		{"prod storage disabled", s3SecretCfg("prod", false, "short"), nil},
		{"dev short key", s3SecretCfg("dev", true, "short"), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePasswords(tc.cfg, tc.cfg.Env == "staging" || tc.cfg.Env == "prod")
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}
