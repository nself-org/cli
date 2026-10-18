package config

// Purpose: Regression test for gap #1 — real-world env var names declared by
//          an actual app (ntask/backend/.env.example) must all be present in
//          knownEnvVars, or userDefinedPrefixes, so `nself build` does not
//          emit false "unknown env var" warnings for legitimate config.
//
// Inputs:  none (static list mirrors the real ntask .env.example var names,
//          captured at fix time — update this list if ntask's env schema
//          changes).
// Outputs: none (t.Error per missing var).
//
// Constraints: This is a snapshot regression guard, not living documentation
//              of ntask's env file — if ntask adds new vars, this test will
//              not catch it until someone updates realWorldAppEnvVars below.
//
// SPORT: cli/internal/config — gap #1 fix (env-schema parity ticket).

import "testing"

// realWorldAppEnvVars mirrors every var declared in
// nself-org/ntask backend/.env.example at the time of the gap #1 fix. Real
// app repos are the ground truth for "legitimate config surface" — this list
// exists so a future edit to knownEnvVars that accidentally drops one of
// these entries fails CI immediately instead of silently reintroducing false
// warnings for ntask (and any other app with a similar env surface).
var realWorldAppEnvVars = []string{
	"ACTION_HANDLER_URL",
	"APNS_KEY_ID", "APNS_KEY_PATH", "APNS_TEAM_ID",
	"AUTH_ACCESS_TOKEN_EXPIRES_IN", "AUTH_ANONYMOUS_USERS_ENABLED",
	"AUTH_CLIENT_URL", "AUTH_DISABLE_NEW_USERS",
	"AUTH_EMAIL_PASSWORDLESS_ENABLED", "AUTH_EMAIL_TEMPLATE_FETCH_URL",
	"AUTH_GRAVATAR_ENABLED", "AUTH_JWT_SECRET", "AUTH_LOCALE_DEFAULT",
	"AUTH_PORT", "AUTH_REFRESH_TOKEN_EXPIRES_IN",
	"AUTH_REQUIRE_EMAIL_VERIFICATION", "AUTH_SERVER_URL",
	"BACKUP_ACCESS_KEY", "BACKUP_RETENTION_DAYS", "BACKUP_S3_BUCKET",
	"BACKUP_S3_ENDPOINT", "BACKUP_S3_PREFIX", "BACKUP_SECRET_KEY",
	"DEMO_SEED", "DR_DATABASE_URL", "FCM_SERVER_KEY",
	"FUNCTIONS_ENABLED", "FUNCTIONS_PORT",
	"HASURA_ADMIN_SECRET", "HASURA_AUTH_EMAIL_SIGNIN_EMAIL_VERIFIED_REQUIRED",
	"HASURA_AUTH_SMTP_HOST", "HASURA_AUTH_SMTP_PASS", "HASURA_AUTH_SMTP_PORT",
	"HASURA_AUTH_SMTP_SECURE", "HASURA_AUTH_SMTP_SENDER", "HASURA_AUTH_SMTP_USER",
	"HASURA_AUTH_URL", "HASURA_GRAPHQL_BATCH_SIZE", "HASURA_GRAPHQL_DEPTH_LIMIT",
	"HASURA_GRAPHQL_DEV_MODE", "HASURA_GRAPHQL_ENABLE_ALLOWLIST",
	"HASURA_GRAPHQL_ENABLE_CONSOLE", "HASURA_GRAPHQL_ENABLE_TELEMETRY",
	"HASURA_GRAPHQL_LIVE_QUERIES_MULTIPLEXED_BATCH_SIZE",
	"HASURA_GRAPHQL_LIVE_QUERIES_MULTIPLEXED_REFETCH_INTERVAL",
	"HASURA_GRAPHQL_NODE_LIMIT", "HASURA_GRAPHQL_UNAUTHORIZED_ROLE",
	"HASURA_PORT",
	"MAILHOG_SMTP_PORT", "MAILHOG_UI_PORT",
	"MINIO_ACCESS_KEY", "MINIO_BUCKET", "MINIO_CONSOLE_PORT",
	"MINIO_ENDPOINT", "MINIO_ENDPOINT_PUBLIC", "MINIO_PORT", "MINIO_SECRET_KEY",
	"POSTGRES_DB", "POSTGRES_PASSWORD", "POSTGRES_PORT", "POSTGRES_USER",
	"RATE_LIMIT_AUTH_RPM", "RATE_LIMIT_GRAPHQL_RPM", "RATE_LIMIT_UPLOADS_RPM",
	"S3_ACCESS_KEY", "S3_BUCKET", "S3_ENDPOINT", "S3_SECRET_KEY",
	"SENTRY_DSN_BACKEND",
	"SMTP_HOST", "SMTP_PASS", "SMTP_PORT", "SMTP_SECURE", "SMTP_SENDER", "SMTP_USER",
	"STORAGE_PORT", "STORAGE_PUBLIC_URL",
	"DOMAIN", "ACME_EMAIL", "AUTH_MODE",
	"SHARED_AUTH_JWT_SECRET", "SHARED_AUTH_SERVER_URL",
}

// TestKnownEnvVars_CoversRealWorldAppSurface verifies that every var in
// realWorldAppEnvVars is present in knownEnvVars (or would be skipped by a
// userDefinedPrefix — none of these currently match one, but the check is
// included for completeness/future-proofing).
func TestKnownEnvVars_CoversRealWorldAppSurface(t *testing.T) {
	knownSet := make(map[string]bool, len(knownEnvVars))
	for _, k := range knownEnvVars {
		knownSet[k] = true
	}

	var missing []string
	for _, v := range realWorldAppEnvVars {
		if knownSet[v] {
			continue
		}
		skippedByPrefix := false
		for _, pfx := range userDefinedPrefixes {
			if len(v) >= len(pfx) && v[:len(pfx)] == pfx {
				skippedByPrefix = true
				break
			}
		}
		if !skippedByPrefix {
			missing = append(missing, v)
		}
	}

	if len(missing) > 0 {
		t.Errorf("knownEnvVars is missing %d real-world app env vars (would trigger false 'unknown env var' warnings): %v", len(missing), missing)
	}
}

// cliWriterEnvVars mirrors every var the CLI's own setup writers generate:
// internal/setup/setup_env_files.go (PGVECTOR_* + bare ADMIN_ENABLED, distinct
// from the already-known NSELF_ADMIN_ENABLED) and internal/setup/envai.go
// (the AI_* family + NSELF_MASTER_SECRET). P6-E11-W2-S1-T4 flagged these
// seven names as absent from knownEnvVars (2026-08-27), causing `nself build`
// to warn "unknown env var — check spelling (ignored)" for config the CLI
// itself generates. Verified 2026-08-31: all seven were already added to
// knownEnvVars by the time this test was written; this is the regression
// guard that would have caught the gap and prevents it recurring.
var cliWriterEnvVars = []string{
	// internal/setup/setup_env_files.go
	"PGVECTOR_ENABLED", "PGVECTOR_DIMENSIONS", "PGVECTOR_HNSW_M",
	"PGVECTOR_HNSW_EF_CONSTRUCTION", "ADMIN_ENABLED",
	// internal/setup/envai.go
	"AI_PROFILE", "AI_DEFAULT_MODEL", "AI_POOL_AUTO_PROVISION",
	"AI_BACKGROUND_LOCAL_ONLY", "AI_DAILY_BUDGET_USD", "AI_TIMEOUT_LOCAL_MS",
	"AI_TIMEOUT_OAUTH_MS", "AI_TIMEOUT_POOL_MS", "AI_TIMEOUT_PAID_MS",
	"AI_POOL_OAUTH_CLIENT_ID", "AI_POOL_OAUTH_CLIENT_SECRET",
	"NSELF_MASTER_SECRET",
}

// TestKnownEnvVars_CoversCLIWriterSurface verifies every var the CLI's own
// setup_env_files.go/envai.go writers generate is present in knownEnvVars —
// see cliWriterEnvVars' doc comment (P6-E11-W2-S1-T4).
func TestKnownEnvVars_CoversCLIWriterSurface(t *testing.T) {
	knownSet := make(map[string]bool, len(knownEnvVars))
	for _, k := range knownEnvVars {
		knownSet[k] = true
	}

	var missing []string
	for _, v := range cliWriterEnvVars {
		if !knownSet[v] {
			missing = append(missing, v)
		}
	}

	if len(missing) > 0 {
		t.Errorf("knownEnvVars is missing %d CLI-writer env vars (would trigger false 'unknown env var' warnings on every `nself build`): %v", len(missing), missing)
	}
}

// hasuraAuthPassthroughEnvVars lists hasura-auth settings that the auth
// service's AUTH_*/HASURA_AUTH_* passthrough (internal/compose/
// core_services_auth_nginx.go) forwards to the container. They work, so they
// must not draw an "unknown env var" warning on every command (D-0918:
// AUTH_JWT_CUSTOM_CLAIMS, the multi-app template's isolation claim mapping).
var hasuraAuthPassthroughEnvVars = []string{
	"AUTH_JWT_CUSTOM_CLAIMS",
	"AUTH_USER_DEFAULT_ROLE", "AUTH_USER_DEFAULT_ALLOWED_ROLES",
	"AUTH_LOCALE_ALLOWED_LOCALES",
	"AUTH_PASSWORD_MIN_LENGTH", "AUTH_PASSWORD_HIBP_ENABLED",
	"AUTH_DISABLE_SIGNUP",
	"AUTH_ACCESS_CONTROL_ALLOWED_EMAILS", "AUTH_ACCESS_CONTROL_ALLOWED_EMAIL_DOMAINS",
	"AUTH_ACCESS_CONTROL_BLOCKED_EMAILS", "AUTH_ACCESS_CONTROL_BLOCKED_EMAIL_DOMAINS",
	"AUTH_MFA_ENABLED", "AUTH_MFA_TOTP_ISSUER",
	"AUTH_WEBAUTHN_RP_NAME", "AUTH_WEBAUTHN_RP_ORIGINS", "AUTH_WEBAUTHN_RP_ID",
}

// TestKnownEnvVars_CoversHasuraAuthPassthrough verifies every hasura-auth
// setting the passthrough honours is in knownEnvVars (D-0918).
func TestKnownEnvVars_CoversHasuraAuthPassthrough(t *testing.T) {
	knownSet := make(map[string]bool, len(knownEnvVars))
	for _, k := range knownEnvVars {
		knownSet[k] = true
	}
	var missing []string
	for _, v := range hasuraAuthPassthroughEnvVars {
		if !knownSet[v] {
			missing = append(missing, v)
		}
	}
	if len(missing) > 0 {
		t.Errorf("knownEnvVars is missing %d hasura-auth passthrough vars (false 'unknown env var' warnings): %v", len(missing), missing)
	}
}
