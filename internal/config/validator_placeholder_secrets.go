package config

// Purpose: fail-closed refusal of known placeholder/example secret values
// (POSTGRES_PASSWORD, HASURA_GRAPHQL_ADMIN_SECRET, AUTH_JWT_SECRET, and the
// same optional secrets validatePasswords already treats as critical) in any
// non-dev environment.
// Inputs: a *Config populated by the loader, after ApplyDefaults has already
// normalized Env (empty becomes "dev"; aliases collapse to dev/staging/prod).
// Outputs: an error naming the offending variable and environment, or nil.
// Constraints: deliberately broader than validatePasswords/validateJWT's
// literal Env=="staging"||"prod" gate — it fires for ANY Env that is not
// exactly "dev", so an unrecognized deploy tier gets no free pass. It is also
// independent of checkPassword's length/pattern gates: a placeholder that
// happens to be long enough, or that predates isInsecurePassword's pattern
// list, is still caught here by exact substring match.
//
// Live finding (verified 2026-09-03): nself-web's Hasura ran in BOTH staging
// and production with HASURA_GRAPHQL_ADMIN_SECRET set to the public
// nself-dev-admin-secret-change-in-prod .env.example placeholder; /v1/metadata
// export succeeded with it. Security-Always-Free doctrine treats a placeholder
// secret in a non-dev environment as a critical finding that blocks deploy.
// This validator does not rotate anything — fixing a live deployment is an
// owner action (rotate the secret, then redeploy).

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

func init() {
	register("placeholder-secrets", validateNoPlaceholderSecrets)
}

// knownPlaceholderSecrets are substrings that, found case-insensitively
// anywhere in a secret value, mark it as an unrotated example/placeholder
// rather than a real credential. Covers the generic CHANGE_ME family in all
// its casing/separator variants plus the specific nself-dev-admin-secret-
// change-in-prod value shipped in web's public .env.example (the
// "change-in-prod" substring alone is enough to catch that one and any
// future sibling placeholder following the same naming convention).
var knownPlaceholderSecrets = []string{
	"change_me",
	"changeme",
	"change-me",
	"change-in-prod",
}

// placeholderSecretField pairs an env-var name with its resolved value for
// placeholder/empty checking.
type placeholderSecretField struct {
	name  string
	value string
}

// secretFileHint tells where to fix a refused secret. .env.secrets loads after
// .env and .env.<env> and wins, so editing .env does nothing for a key that
// file already holds (build persists generated credentials there, D-0708).
const secretFileHint = "in the file that defines it (.env.secrets overrides .env and .env.<env>)"

// validateNoPlaceholderSecrets refuses to proceed when cfg.Env is not "dev"
// and any critical secret is empty or matches a known placeholder value.
// Dev is exempt: placeholders there are expected and must keep working
// exactly as today.
func validateNoPlaceholderSecrets(cfg *Config) error {
	if cfg.Env == "dev" {
		return nil
	}

	// Always-required secrets: reuse the same three fields validatePasswords
	// treats as critical regardless of environment (POSTGRES_PASSWORD,
	// HASURA_GRAPHQL_ADMIN_SECRET) plus AUTH_JWT_SECRET, which loader_parse_env.go
	// aliases onto cfg.Hasura.JWTKey (see loader_parse_env.go:94).
	required := []placeholderSecretField{
		{"POSTGRES_PASSWORD", cfg.Postgres.Password},
		{"HASURA_GRAPHQL_ADMIN_SECRET", cfg.Hasura.AdminSecret},
		{"AUTH_JWT_SECRET", cfg.Hasura.JWTKey},
	}
	for _, f := range required {
		if f.value == "" {
			return fmt.Errorf("%s is empty in %s environment — set a unique value %s: %w",
				f.name, cfg.Env, secretFileHint, errs.ErrPlaceholderSecret)
		}
		if isPlaceholderSecret(f.value) {
			return fmt.Errorf("%s is a known placeholder value in %s environment — set a unique value %s: %w",
				f.name, cfg.Env, secretFileHint, errs.ErrPlaceholderSecret)
		}
	}

	// Optional secrets: only checked when their service is enabled, mirroring
	// validatePasswords' own conditions. Empty is left alone here (an empty
	// optional secret is validatePasswords' concern, e.g. Redis no-auth);
	// this validator only refuses a placeholder VALUE, never an absent one,
	// for these fields.
	optional := []placeholderSecretField{}
	if cfg.Redis.Enabled && cfg.Redis.Password != "" {
		optional = append(optional, placeholderSecretField{"REDIS_PASSWORD", cfg.Redis.Password})
	}
	if cfg.Minio.Enabled && cfg.Minio.RootPassword != "" {
		optional = append(optional, placeholderSecretField{"MINIO_ROOT_PASSWORD", cfg.Minio.RootPassword})
	}
	if cfg.Search.Enabled && strings.EqualFold(cfg.Search.Engine, "meilisearch") && cfg.Search.MeiliSearch.MasterKey != "" {
		optional = append(optional, placeholderSecretField{"MEILISEARCH_MASTER_KEY", cfg.Search.MeiliSearch.MasterKey})
	}
	if cfg.Monitoring.Enabled && cfg.Monitoring.GrafanaAdminPassword != "" {
		optional = append(optional, placeholderSecretField{"GRAFANA_ADMIN_PASSWORD", cfg.Monitoring.GrafanaAdminPassword})
	}
	for _, f := range optional {
		if isPlaceholderSecret(f.value) {
			return fmt.Errorf("%s is a known placeholder value in %s environment — set a unique value %s: %w",
				f.name, cfg.Env, secretFileHint, errs.ErrPlaceholderSecret)
		}
	}

	return nil
}

// isPlaceholderSecret reports whether value contains a known placeholder
// substring, case-insensitively.
func isPlaceholderSecret(value string) bool {
	lower := strings.ToLower(value)
	for _, p := range knownPlaceholderSecrets {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}
