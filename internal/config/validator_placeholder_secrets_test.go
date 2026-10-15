package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

// validPlaceholderTestConfig returns a *Config with strong, non-placeholder
// values for every secret validateNoPlaceholderSecrets inspects, so a single
// field can be overridden per test case without tripping on the others.
// The values below deliberately avoid every substring in insecurePatterns
// (validator.go) — "password", "secret", "admin", etc. — so a test built on
// this base config also passes the pre-existing "passwords" validator when
// run through runAll, isolating failures to the placeholder-secrets check
// under test. They are also deliberately low-entropy (concatenated dictionary
// words, no digit/case mixing) rather than random-looking strings, so they
// read clearly as inert test fixtures and don't trip gitleaks' generic-api-key
// entropy heuristic the way a random alnum string would.
func validPlaceholderTestConfig(env string) *Config {
	return &Config{
		Env:         env,
		ProjectName: "test-project",
		BaseDomain:  "example.com",
		Postgres: PostgresConfig{
			Password: "actuallyuniquevalueforthisenvironmentnumberone",
		},
		Hasura: HasuraConfig{
			AdminSecret: "totallydifferentvaluealtogetherforthisservice",
			JWTKey:      "completelyseparatevalueforauthenticationtoken",
		},
	}
}

// TestValidateNoPlaceholderSecrets is the table-driven proof for the
// fail-closed placeholder-secret gate: dev is exempt, every non-dev tier
// refuses a known placeholder or an empty critical secret, and a real value
// passes.
func TestValidateNoPlaceholderSecrets(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(cfg *Config)
		env     string
		wantErr bool
		wantVar string // substring the error must name, when wantErr is true
	}{
		{
			name: "dev with CHANGE_ME placeholder passes untouched",
			env:  "dev",
			mutate: func(cfg *Config) {
				cfg.Postgres.Password = "CHANGE_ME"
				cfg.Hasura.AdminSecret = "CHANGE_ME"
				cfg.Hasura.JWTKey = "CHANGE_ME"
			},
			wantErr: false,
		},
		{
			name: "staging with the live nself-web placeholder fails naming the var",
			env:  "staging",
			mutate: func(cfg *Config) {
				cfg.Hasura.AdminSecret = "nself-dev-admin-secret-change-in-prod"
			},
			wantErr: true,
			wantVar: "HASURA_GRAPHQL_ADMIN_SECRET",
		},
		{
			name: "prod with empty POSTGRES_PASSWORD fails",
			env:  "prod",
			mutate: func(cfg *Config) {
				cfg.Postgres.Password = ""
			},
			wantErr: true,
			wantVar: "POSTGRES_PASSWORD",
		},
		{
			name:    "prod with real values passes",
			env:     "prod",
			mutate:  func(cfg *Config) {},
			wantErr: false,
		},
		{
			name: "staging with a change-in-prod substring fails even without an exact match",
			env:  "staging",
			mutate: func(cfg *Config) {
				cfg.Hasura.JWTKey = "some-other-service-secret-change-in-prod-suffix"
			},
			wantErr: true,
			wantVar: "AUTH_JWT_SECRET",
		},
		{
			name: "unrecognized non-dev tier still refuses a placeholder (fail-closed)",
			env:  "canary",
			mutate: func(cfg *Config) {
				cfg.Postgres.Password = "changeme"
			},
			wantErr: true,
			wantVar: "POSTGRES_PASSWORD",
		},
		{
			name: "staging with CHANGE_ME (underscore, uppercase) fails",
			env:  "staging",
			mutate: func(cfg *Config) {
				cfg.Postgres.Password = "CHANGE_ME"
			},
			wantErr: true,
			wantVar: "POSTGRES_PASSWORD",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validPlaceholderTestConfig(tc.env)
			tc.mutate(cfg)

			err := validateNoPlaceholderSecrets(cfg)

			if tc.wantErr && err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if tc.wantErr {
				if !errors.Is(err, errs.ErrPlaceholderSecret) {
					t.Errorf("error does not wrap errs.ErrPlaceholderSecret: %v", err)
				}
				if tc.wantVar != "" && !strings.Contains(err.Error(), tc.wantVar) {
					t.Errorf("error %q does not name expected variable %q", err.Error(), tc.wantVar)
				}
				if !strings.Contains(err.Error(), tc.env) {
					t.Errorf("error %q does not name the environment %q", err.Error(), tc.env)
				}
			}
		})
	}
}

// TestIsPlaceholderSecret exercises the substring matcher directly across
// casing and separator variants, plus a real-looking value that must NOT match.
func TestIsPlaceholderSecret(t *testing.T) {
	placeholders := []string{
		"CHANGE_ME",
		"changeme",
		"Change-Me",
		"change-me",
		"nself-dev-admin-secret-change-in-prod",
		"prefix-CHANGE_ME-suffix",
		"something-CHANGE-IN-PROD-value",
	}
	for _, v := range placeholders {
		if !isPlaceholderSecret(v) {
			t.Errorf("isPlaceholderSecret(%q) = false, want true", v)
		}
	}

	real := []string{
		"totallydifferentvaluealtogetherforthisservice",
		"completelyseparatevalueforauthenticationtoken",
		"",
	}
	for _, v := range real {
		if isPlaceholderSecret(v) {
			t.Errorf("isPlaceholderSecret(%q) = true, want false", v)
		}
	}
}

// TestValidateNoPlaceholderSecretsWiredIntoRunAll proves the check actually
// runs as part of the full validator registry (the path nself start/deploy
// invoke via config.Validate), not just as a standalone function.
func TestValidateNoPlaceholderSecretsWiredIntoRunAll(t *testing.T) {
	cfg := validPlaceholderTestConfig("staging")
	cfg.Hasura.AdminSecret = "nself-dev-admin-secret-change-in-prod"

	err := runAll(cfg)
	if err == nil {
		t.Fatal("expected runAll to fail for a staging config with a placeholder Hasura admin secret")
	}
	if !strings.Contains(err.Error(), "placeholder-secrets") {
		t.Errorf("runAll error %q does not attribute the failure to the placeholder-secrets validator", err.Error())
	}
	if !strings.Contains(err.Error(), "HASURA_GRAPHQL_ADMIN_SECRET") {
		t.Errorf("runAll error %q does not name HASURA_GRAPHQL_ADMIN_SECRET", err.Error())
	}
}

// TestValidateNoPlaceholderSecretsPointsAtEnvSecrets pins D-0708: nself build
// persists generated credentials to .env.secrets, which loads after .env, so a
// hint that says "set it in .env" sends the user to a file that cannot win.
func TestValidateNoPlaceholderSecretsPointsAtEnvSecrets(t *testing.T) {
	for _, mutate := range []func(*Config){
		func(c *Config) { c.Postgres.Password = "" },
		func(c *Config) { c.Postgres.Password = "change_me" },
	} {
		cfg := validPlaceholderTestConfig("prod")
		mutate(cfg)
		err := validateNoPlaceholderSecrets(cfg)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), ".env.secrets") {
			t.Errorf("error %q does not point at .env.secrets", err.Error())
		}
	}
}
