package config

// loader_known_vars_core.go — core + service env var names (Core, Postgres,
// Hasura, Auth, Nginx, SSL, Redis). Split from loader_known_vars.go (T-P6-E2-W1-S1-T3).
// Purpose: first quarter of the knownEnvVars list, combined in loader_known_vars.go.
// Inputs:  none. Outputs: knownEnvVarsCore []string.
// Constraints: keep entries verbatim and in original order; see loader_known_vars.go header.

var knownEnvVarsCore = []string{
	// Core
	"PROJECT_NAME",
	"BASE_DOMAIN",
	"PROJECT_DOMAIN",
	"ENV",
	"PROJECT_DESCRIPTION",
	"ADMIN_EMAIL",
	"DB_ENV_SEEDS",
	// APP_NAME: opt-in subdomain prefix for multi-app nginx routing (gap #5).
	"APP_NAME",
	// DOMAIN/ACME_EMAIL are the Traefik/staging-prod convention some app repos
	// use alongside (or instead of) BASE_DOMAIN — see ntask backend/.env.example.
	// Not read by the CLI loader directly; listed to suppress false warnings.
	"DOMAIN",
	"ACME_EMAIL",
	// App-level demo/seed toggle (read by app seed scripts, not the CLI loader).
	"DEMO_SEED",
	// IMAGE_PINNING: lock|legacy image reference mode (P7-LIVE-17, ADR 0030).
	"IMAGE_PINNING",

	// PostgreSQL
	"POSTGRES_VERSION",
	"POSTGRES_IMAGE",
	"POSTGRES_HOST",
	"POSTGRES_PORT",
	"POSTGRES_DB",
	"POSTGRES_USER",
	"POSTGRES_PASSWORD",
	"POSTGRES_EXTENSIONS",
	"POSTGRES_EXPOSE_PORT",
	"POSTGRES_MEM_LIMIT",
	"POSTGRES_CPU_LIMIT",
	// pgvector extension toggles, written by internal/setup/setup_env_files.go
	// (gap #4, msg-2026-04-30-env-var-warnings-on-build.md).
	"PGVECTOR_ENABLED",
	"PGVECTOR_DIMENSIONS",
	"PGVECTOR_HNSW_M",
	"PGVECTOR_HNSW_EF_CONSTRUCTION",

	// Hasura
	"HASURA_VERSION",
	"HASURA_GRAPHQL_ADMIN_SECRET",
	// HASURA_ADMIN_SECRET is a commonly-used alias for HASURA_GRAPHQL_ADMIN_SECRET
	// seen in real app .env files (e.g. ntask/backend/.env.example). The loader
	// does not read it directly today; listed here to suppress false warnings.
	"HASURA_ADMIN_SECRET",
	"HASURA_JWT_KEY",
	"HASURA_JWT_TYPE",
	"HASURA_GRAPHQL_ENABLE_CONSOLE",
	"HASURA_GRAPHQL_DEV_MODE",
	"HASURA_DEV_MODE",
	"HASURA_GRAPHQL_CORS_DOMAIN",
	"HASURA_GRAPHQL_JWT_SECRET",
	"HASURA_ROUTE",
	"HASURA_PORT",
	"HASURA_MEM_LIMIT",
	"HASURA_CPU_LIMIT",
	"HASURA_GRAPHQL_LOG_LEVEL",
	// Real-world Hasura tuning vars declared by apps (not yet read by the
	// loader as typed fields, but legitimate Hasura engine config surface).
	"HASURA_GRAPHQL_ENABLE_TELEMETRY",
	"HASURA_GRAPHQL_ENABLE_ALLOWLIST",
	"HASURA_GRAPHQL_UNAUTHORIZED_ROLE",
	"HASURA_GRAPHQL_NODE_LIMIT",
	"HASURA_GRAPHQL_DEPTH_LIMIT",
	"HASURA_GRAPHQL_BATCH_SIZE",
	"HASURA_GRAPHQL_LIVE_QUERIES_MULTIPLEXED_BATCH_SIZE",
	"HASURA_GRAPHQL_LIVE_QUERIES_MULTIPLEXED_REFETCH_INTERVAL",
	// Hasura Actions/cron webhook callback base URL (gap #11: wired into the
	// generated Hasura service env so Actions can call back into `functions`).
	"ACTION_HANDLER_URL",

	// Auth
	// AUTH_ENABLED is written by internal/setup/setup_env_files.go (gap #4,
	// msg-2026-04-30-env-var-warnings-on-build.md) but has no AuthConfig
	// struct field — it toggles whether the auth service is generated at all,
	// checked ahead of the per-field AuthConfig parse.
	"AUTH_ENABLED",
	"AUTH_VERSION",
	"AUTH_PORT",
	"AUTH_CLIENT_URL",
	"AUTH_ACCESS_TOKEN_EXPIRES_IN",
	"AUTH_REFRESH_TOKEN_EXPIRES_IN",
	"AUTH_ROUTE",
	"AUTH_SMTP_HOST",
	"AUTH_SMTP_PORT",
	"AUTH_SMTP_USER",
	"AUTH_SMTP_PASS",
	"AUTH_SMTP_SECURE",
	"AUTH_SMTP_SENDER",
	"AUTH_MEM_LIMIT",
	"AUTH_CPU_LIMIT",
	"AUTH_EXTRA_REDIRECT_URLS",
	"AUTH_RATE_LIMIT",
	"AUTH_WEBAUTHN_ENABLED",
	"AUTH_LOG_LEVEL",
	// Auth-mode toggle (bundled vs external shared auth — see AUTH_MODE doctrine
	// in app .env files) and the shared-auth vars used in external mode.
	"AUTH_MODE",
	"SHARED_AUTH_JWT_SECRET",
	"SHARED_AUTH_SERVER_URL",
	// hasura-auth container feature flags declared by real app .env files.
	"AUTH_ANONYMOUS_USERS_ENABLED",
	"AUTH_DISABLE_NEW_USERS",
	"AUTH_EMAIL_PASSWORDLESS_ENABLED",
	"AUTH_REQUIRE_EMAIL_VERIFICATION",
	"AUTH_EMAIL_TEMPLATE_FETCH_URL",
	"AUTH_GRAVATAR_ENABLED",
	"AUTH_LOCALE_DEFAULT",
	// hasura-auth internal URL (used by app functions to call the auth admin API).
	"HASURA_AUTH_URL",
	// hasura-auth SMTP vars (distinct prefix from AUTH_SMTP_* — hasura-auth
	// reads these directly; the CLI passes them through unmodified).
	"HASURA_AUTH_SMTP_HOST",
	"HASURA_AUTH_SMTP_PORT",
	"HASURA_AUTH_SMTP_USER",
	"HASURA_AUTH_SMTP_PASS",
	"HASURA_AUTH_SMTP_SENDER",
	"HASURA_AUTH_SMTP_SECURE",
	"HASURA_AUTH_EMAIL_SIGNIN_EMAIL_VERIFIED_REQUIRED",

	// Nginx
	"NGINX_VERSION",
	"NGINX_HTTP_PORT",
	"NGINX_PORT",
	"NGINX_HTTPS_PORT",
	"NGINX_SSL_PORT",
	"NGINX_CLIENT_MAX_BODY_SIZE",
	"NGINX_BIND_IP",
	// Names the stack whose nginx fronts this project's domains, when
	// this project has no ingress nginx of its own — see
	// NginxConfig.FrontedBy. Listed here so setting it does not emit a
	// false "unknown env var" warning on every build.
	"NGINX_FRONTED_BY",
	// Per-zone rate limit overrides — read by parseEnvToConfig into
	// NginxConfig.RateLimitAPI/RateLimitAuth/RateLimitAI but were missing from
	// this schema list (pre-existing gap, fixed alongside gap #1).
	"RATE_LIMIT_API_RPS",
	"RATE_LIMIT_AUTH_RPS",
	"RATE_LIMIT_AI_RPS",
	// RPM-suffixed variants seen in real app .env files (e.g. ntask). Not read
	// by the CLI loader (the RPS vars above are canonical); listed to suppress
	// false warnings.
	"RATE_LIMIT_AUTH_RPM",
	"RATE_LIMIT_GRAPHQL_RPM",
	"RATE_LIMIT_UPLOADS_RPM",

	// SSL
	"SSL_MODE",
	"EXTRA_SSL_DOMAINS",

	// Redis
	"REDIS_ENABLED",
	"REDIS_VERSION",
	"REDIS_PORT",
	"REDIS_PASSWORD",
	"REDIS_MEMORY",
	"REDIS_CPU",
}
