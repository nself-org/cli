package config

// loader_known_vars_ops.go — ops/plugin env var names (Backup, Disaster
// Recovery, Plugin Pro, Plugin System, Docker, Start/Stop, plugin-managed
// compose vars, CLI tool behavior vars). Split from loader_known_vars.go
// (T-P6-E2-W1-S1-T3).
// Purpose: third quarter of the knownEnvVars list, combined in loader_known_vars.go.
// Inputs:  none. Outputs: knownEnvVarsOps []string.
// Constraints: keep entries verbatim and in original order; see loader_known_vars.go header.

var knownEnvVarsOps = []string{
	// Backup
	"BACKUP_ENABLED",
	"BACKUP_DIR",
	"BACKUP_SCHEDULE",
	"BACKUP_RETENTION_DAYS",
	"BACKUP_CLOUD_PROVIDER",
	"BACKUP_REMOTE",
	"BACKUP_ENCRYPTION",
	"BACKUP_AGE_RECIPIENTS",
	"BACKUP_SCHEDULE_FULL",
	"BACKUP_WAL_INTERVAL_SECONDS",
	"BACKUP_RETENTION_DAILY",
	"BACKUP_RETENTION_WEEKLY",
	"BACKUP_RETENTION_MONTHLY",
	"BACKUP_RESTORE_TEST_SCHEDULE",
	"BACKUP_ALERT_ON_FAILURE",
	"BACKUP_S3_ACCESS_KEY_ID",
	"BACKUP_S3_SECRET_ACCESS_KEY",
	"BACKUP_S3_REGION",
	"BACKUP_S3_ENDPOINT",
	"BACKUP_CRITICAL_TABLES",
	// BACKUP_ACCESS_KEY / BACKUP_SECRET_KEY: shorter aliases for
	// BACKUP_S3_ACCESS_KEY_ID / BACKUP_S3_SECRET_ACCESS_KEY, used verbatim by
	// real project .env files (e.g. ntask/backend/.env.example, R2-backed).
	// These were previously "known" (no warning) but silently unread — remote
	// backup upload configured this way did nothing. loader_parse_env_ops.go
	// now accepts either name via firstNonEmpty(), canonical name winning if
	// both are set. BACKUP_S3_BUCKET / BACKUP_S3_PREFIX remain app-level
	// (consumed by app backup scripts, not this CLI's config struct).
	"BACKUP_ACCESS_KEY",
	"BACKUP_SECRET_KEY",
	"BACKUP_S3_BUCKET",
	"BACKUP_S3_PREFIX",

	// Disaster Recovery
	"DR_SECONDARY_REGION",
	"DR_STANDBY_HOST",
	"DR_DRILL_SCHEDULE",
	// DR_DATABASE_URL is the standby/replica connection string used by app-level
	// DR tooling; not read by the CLI loader.
	"DR_DATABASE_URL",

	// Plugin Pro
	"NOTIFY_INTERNAL_SECRET",
	"NOTIFY_PORT",
	"NOTIFY_VAPID_PUBLIC_KEY",
	"NOTIFY_VAPID_PRIVATE_KEY",
	"NOTIFY_ROUTE",
	"CRON_INTERNAL_SECRET",
	"CRON_PORT",
	"CRON_RETENTION_DAYS",
	"PLUGIN_AI_MEMORY_LIMIT",
	"PLUGIN_AI_CPU_LIMIT",
	"PLUGIN_MUX_MEMORY_LIMIT",
	"PLUGIN_MUX_CPU_LIMIT",
	"PLUGIN_CLAW_MEMORY_LIMIT",
	"PLUGIN_CLAW_CPU_LIMIT",
	"PLUGIN_DEFAULT_MEMORY_LIMIT",
	"PLUGIN_DEFAULT_CPU_LIMIT",
	"PLUGIN_INTERNAL_SECRET",

	// Plugin System
	"NSELF_PLUGIN_DIR",
	"NSELF_PLUGIN_CACHE",
	"NSELF_PLUGIN_REGISTRY",
	"NSELF_REGISTRY_CACHE_TTL",
	"NSELF_PLUGIN_LICENSE_KEY",
	"NSELF_LICENSE_SKIP_VERIFY",
	"NSELF_PING_API_URL",
	"NSELF_PRICING_URL",

	// Docker
	"DOCKER_NETWORK",
	"DOCKER_LOG_MAX_SIZE",
	"DOCKER_LOG_MAX_FILE",
	"DOCKER_STOP_GRACE_PERIOD",
	"NSELF_DOCKER_BUILD_TIMEOUT",

	// Start/Stop
	"NSELF_START_MODE",
	"NSELF_HEALTH_CHECK_TIMEOUT",
	"NSELF_HEALTH_CHECK_INTERVAL",
	"NSELF_HEALTH_CHECK_REQUIRED",
	"NSELF_CLEANUP_ON_START",
	"NSELF_ALLOW_EXPOSED_PORTS",
	"NSELF_PARALLEL_LIMIT",
	"NSELF_LOG_LEVEL",
	"NSELF_SKIP_HEALTH_CHECKS",
	"NSELF_STOP_TIMEOUT",

	// Plugin-managed: compose-injected vars that users may set in .env.
	// The CLI loader does not read these; they are listed here only to suppress
	// false "unknown env var" warnings from WarnUnknownEnvVars.
	// Auth service (nHost auth container) — passed through compose template.
	// NOTE: AUTH_JWT_SECRET and AUTH_JWT_KEY ARE read directly by
	// parseEnvToConfig (gap #4 fix) as fallback sources for cfg.Hasura.JWTKey,
	// so a user-declared value survives every rebuild instead of being
	// silently ignored and regenerated.
	"AUTH_HOST",
	"AUTH_SERVER_URL",
	"AUTH_JWT_SECRET",
	"AUTH_JWT_KEY",
	"AUTH_REFRESH_TOKEN_SECRET",
	"AUTH_ACCESS_TOKEN_EXPIRY",
	"AUTH_REFRESH_TOKEN_EXPIRY",
	"AUTH_EMAIL_SIGNIN_EMAIL_VERIFIED_REQUIRED",
	// Hasura container — passed through compose template.
	"HASURA_GRAPHQL_ENABLE_TELEMETRY",
	"HASURA_GRAPHQL_UNAUTHORIZED_ROLE",
	"HASURA_CONSOLE_PORT",
	"HASURA_GRAPHQL_JWT_SECRET",
	"HASURA_GRAPHQL_DATABASE_URL",
	"HASURA_METADATA_DATABASE_URL",
	// Nginx compose template vars.
	"NGINX_GZIP_ENABLED",
	"NGINX_MODE",
	"NGINX_MEM_LIMIT",
	// MinIO/Storage — compose-computed.
	"S3_ENDPOINT",
	"STORAGE_PORT",
	"FILES_ROUTE",
	// nSelf Admin container.
	"NSELF_ADMIN_USER",
	"NSELF_ADMIN_PASSWORD",
	// Typesense search provider (partial: TYPESENSE_PORT/ROUTE not in knownEnvVars struct).
	"TYPESENSE_PORT",
	"TYPESENSE_ROUTE",
	// Notify plugin.
	"NOTIFY_VAPID_SUBJECT",
	// Docker Compose runtime vars (exported by shell wrapper, not read by loader).
	"COMPOSE_PROJECT_NAME",
	"DOCKER_BUILDKIT",
	// Phase 14 CLI-command vars (read by cmd handlers, not by loader).
	"NSELF_AUTO_TRUST_CA",
	"NSELF_AUTO_HOSTS_ENTRIES",
	"NSELF_MKCERT_CAROOT",
	"NSELF_NO_MONOREPO",
	// CLI tool behavior vars (read by main binary, not by loader).
	"DEBUG",
	"NO_COLOR",
	// Postgres internal port (documentation-only; always 5432).
	"POSTGRES_INTERNAL_PORT",

	// AI-tier config block, written to .env.secrets by internal/setup/envai.go
	// (CLI-R18) — not read via a struct env tag, hence absent here until gap #4
	// (msg-2026-04-30-env-var-warnings-on-build.md).
	"AI_PROFILE",
	"AI_AUTO_INSTALL",
	"AI_DEFAULT_MODEL",
	"AI_EMBEDDING_MODEL",
	"AI_POOL_AUTO_PROVISION",
	"AI_BACKGROUND_LOCAL_ONLY",
	"AI_DAILY_BUDGET_USD",
	"AI_MONTHLY_BUDGET_USD",
	"AI_TIMEOUT_LOCAL_MS",
	"AI_TIMEOUT_OAUTH_MS",
	"AI_TIMEOUT_POOL_MS",
	"AI_TIMEOUT_PAID_MS",
	"AI_POOL_OAUTH_CLIENT_ID",
	"AI_POOL_OAUTH_CLIENT_SECRET",
	// KEK for OAuth/API-key encryption in the zero-config AI pool — see
	// internal/setup/envai.go's anti-clobber guarantee.
	"NSELF_MASTER_SECRET",
}
