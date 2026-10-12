package config

// loader_known_vars_storage.go — storage/messaging env var names (MinIO,
// Mailpit/Mailhog, Functions, MLflow, Admin, Search, Monitoring, Email).
// Split from loader_known_vars.go (T-P6-E2-W1-S1-T3).
// Purpose: second quarter of the knownEnvVars list, combined in loader_known_vars.go.
// Inputs:  none. Outputs: knownEnvVarsStorage []string.
// Constraints: keep entries verbatim and in original order; see loader_known_vars.go header.

var knownEnvVarsStorage = []string{
	// Core object storage
	"OBJECT_STORAGE_ENABLED",
	"OBJECT_STORAGE_BUCKETS",
	"SEAWEEDFS_PORT",
	"SEAWEEDFS_VERSION",

	// MinIO / Storage
	"MINIO_ENABLED",
	"STORAGE_ENABLED",
	"MINIO_VERSION",
	"MINIO_PORT",
	"MINIO_CONSOLE_PORT",
	"MINIO_ROOT_USER",
	"MINIO_ROOT_PASSWORD",
	// MINIO_ACCESS_KEY/MINIO_SECRET_KEY are documented aliases for
	// MINIO_ROOT_USER/MINIO_ROOT_PASSWORD (gap #2). The loader aliases them in
	// parseEnvToConfig when the ROOT_* vars are not already set.
	"MINIO_ACCESS_KEY",
	"MINIO_SECRET_KEY",
	"MINIO_DEFAULT_BUCKETS",
	"MINIO_REGION",
	"S3_ACCESS_KEY",
	"S3_SECRET_KEY",
	"S3_BUCKET",
	// MINIO_BUCKET is an app-level alias for S3_BUCKET seen in real .env files;
	// not read by the CLI loader (apps use it directly), listed to suppress
	// false warnings.
	"MINIO_BUCKET",
	// MINIO_ENDPOINT/MINIO_ENDPOINT_PUBLIC are app-level MinIO endpoint URLs
	// (internal Docker network + public presigned-URL host). Compose-computed
	// equivalents exist as S3_ENDPOINT; these are read directly by apps.
	"MINIO_ENDPOINT",
	"MINIO_ENDPOINT_PUBLIC",
	"STORAGE_VERSION",
	"STORAGE_ROUTE",
	"STORAGE_CONSOLE_ROUTE",
	// STORAGE_PUBLIC_URL is the externally-reachable storage URL apps use for
	// generating download links; STORAGE_PORT is already compose-computed below.
	"STORAGE_PUBLIC_URL",
	"MINIO_MEMORY",
	"MINIO_CPU",

	// Mailpit
	"MAILPIT_ENABLED",
	"MAILPIT_VERSION",
	"MAILPIT_SMTP_PORT",
	"MAILPIT_UI_PORT",
	"MAILPIT_MAX_MESSAGES",
	"MAILPIT_ROUTE",
	"MAIL_ROUTE",
	"MAILPIT_UI_USER",
	"MAILPIT_UI_PASSWORD",
	// Mailhog — legacy predecessor to Mailpit, still referenced by older app
	// .env files (e.g. ntask). Not read by the CLI loader.
	"MAILHOG_SMTP_PORT",
	"MAILHOG_UI_PORT",

	// Functions
	"FUNCTIONS_ENABLED",
	"FUNCTIONS_VERSION",
	"FUNCTIONS_PORT",
	"FUNCTIONS_ROUTE",

	// MLflow
	"MLFLOW_ENABLED",
	"MLFLOW_VERSION",
	"MLFLOW_PORT",
	"MLFLOW_ROUTE",
	"MLFLOW_DB_NAME",
	"MLFLOW_ARTIFACTS_BUCKET",
	"MLFLOW_AUTH_ENABLED",
	"MLFLOW_AUTH_USERNAME",
	"MLFLOW_AUTH_PASSWORD",

	// Admin
	"NSELF_ADMIN_ENABLED",
	"NSELF_ADMIN_VERSION",
	"NSELF_ADMIN_PORT",
	// Bare ADMIN_ENABLED is a distinct, non-NSELF_-prefixed form also written
	// by internal/setup/setup_env_files.go (gap #4,
	// msg-2026-04-30-env-var-warnings-on-build.md) — both forms exist in the
	// codebase and must both be recognized.
	"ADMIN_ENABLED",
	"NSELF_ADMIN_ROUTE",
	"NSELF_ADMIN_DEV",
	"NSELF_ADMIN_DEV_PORT",
	"ADMIN_SECRET_KEY",
	"ADMIN_PASSWORD_HASH",

	// Search
	"SEARCH_ENABLED",
	"SEARCH_ENGINE",
	"SEARCH_PROVIDER",
	"SEARCH_PORT",
	"SEARCH_API_KEY",
	"SEARCH_ROUTE",
	"SEARCH_INDEX_PREFIX",
	"SEARCH_AUTO_INDEX",
	"SEARCH_LANGUAGE",
	"MEILISEARCH_VERSION",
	"MEILISEARCH_MASTER_KEY",
	"MEILISEARCH_ENV",
	"MEILISEARCH_WARMUP_QUERIES",
	"MEILI_ENV",
	"TYPESENSE_VERSION",
	"TYPESENSE_API_KEY",
	"TYPESENSE_ENABLE_CORS",
	"TYPESENSE_LOG_LEVEL",
	"TYPESENSE_NUM_MEMORY_SHARDS",
	"TYPESENSE_SNAPSHOT_INTERVAL_SECONDS",
	"ELASTICSEARCH_VERSION",
	"ELASTICSEARCH_PORT",
	"ELASTICSEARCH_PASSWORD",
	"ELASTICSEARCH_MEMORY",

	// Monitoring
	"MONITORING_ENABLED",
	"PROMETHEUS_ENABLED",
	"PROMETHEUS_PORT",
	"GRAFANA_ENABLED",
	"GRAFANA_PORT",
	"GRAFANA_ADMIN_USER",
	"GRAFANA_ADMIN_PASSWORD",
	"GRAFANA_ROUTE",
	"LOKI_ENABLED",
	"LOKI_PORT",
	"PROMTAIL_ENABLED",
	"TEMPO_ENABLED",
	"TEMPO_PORT",
	"ALERTMANAGER_ENABLED",
	"ALERTMANAGER_PORT",
	"CADVISOR_ENABLED",
	"CADVISOR_PORT",
	"NODE_EXPORTER_ENABLED",
	"NODE_EXPORTER_PORT",
	"POSTGRES_EXPORTER_ENABLED",
	"POSTGRES_EXPORTER_PORT",
	"REDIS_EXPORTER_ENABLED",
	"REDIS_EXPORTER_PORT",

	// Email
	"EMAIL_PROVIDER",
	"EMAIL_FROM",
	"ELASTIC_EMAIL_API_KEY",
	"ELASTIC_EMAIL_ACCOUNT_EMAIL",
	"SENDGRID_API_KEY",
	"POSTMARK_API_KEY",
	"MAILGUN_API_KEY",
	"MAILGUN_DOMAIN",
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_REGION",
	"SMTP_HOST",
	"SMTP_PORT",
	"SMTP_USER",
	"SMTP_PASS",
	"SMTP_SECURE",
	// SMTP_SENDER is the "From" address companion to SMTP_* above, used
	// directly by app mailers (distinct from the CLI's EMAIL_FROM field).
	"SMTP_SENDER",
}
