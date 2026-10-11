package config

// KnownEnvVars returns the list of environment variable names that the
// CLI loader reads. Used by commands like `nself config list` to display
// known vars with their current values.
func KnownEnvVars() []string {
	return knownEnvVars
}

// DefaultFor returns the documented default value for an env var, as a
// human-readable string. Returns "" when the var has no static default
// (e.g. generated secrets, or dynamic per-env defaults).
func DefaultFor(key string) string {
	defaults := map[string]string{
		// Core
		"PROJECT_NAME": "myproject",
		"BASE_DOMAIN":  "local.nself.org",
		"ENV":          "dev",
		"DB_ENV_SEEDS": "true",

		// PostgreSQL
		"POSTGRES_VERSION":     "16-alpine",
		"POSTGRES_HOST":        "postgres",
		"POSTGRES_PORT":        "5432",
		"POSTGRES_DB":          "nself",
		"POSTGRES_USER":        "postgres",
		"POSTGRES_EXPOSE_PORT": "auto",
		"POSTGRES_MEM_LIMIT":   "2g",
		"POSTGRES_CPU_LIMIT":   "2.0",
		"POSTGRES_EXTENSIONS":  "uuid-ossp",

		// Hasura
		"HASURA_VERSION":   "v2.44.0",
		"HASURA_JWT_TYPE":  "HS256", // JWT-ALGO-01: HS256 default — matches auto-generated JWTKey; RS256 requires user-supplied key material
		"HASURA_ROUTE":     "api",
		"HASURA_PORT":      "8080",
		"HASURA_MEM_LIMIT": "1g",
		"HASURA_CPU_LIMIT": "1.0",

		// Auth
		"AUTH_VERSION":                  "0.36.0",
		"AUTH_PORT":                     "4000",
		"AUTH_CLIENT_URL":               "http://localhost:3000",
		"AUTH_ACCESS_TOKEN_EXPIRES_IN":  "900",
		"AUTH_REFRESH_TOKEN_EXPIRES_IN": "2592000",
		"AUTH_ROUTE":                    "auth",
		"AUTH_SMTP_HOST":                "mailpit",
		"AUTH_SMTP_PORT":                "1025",
		"AUTH_MEM_LIMIT":                "256m",
		"AUTH_CPU_LIMIT":                "0.25",
		"AUTH_RATE_LIMIT":               "30r/m",

		// Nginx
		"NGINX_VERSION":              "alpine",
		"NGINX_HTTP_PORT":            "80",
		"NGINX_HTTPS_PORT":           "443",
		"NGINX_CLIENT_MAX_BODY_SIZE": "100M",

		// SSL
		"SSL_MODE": "local (dev) / letsencrypt (prod)",

		// Redis
		"REDIS_ENABLED":   "false",
		"REDIS_VERSION":   "7-alpine",
		"REDIS_PORT":      "6379",
		"REDIS_MEMORY":    "512M",
		"REDIS_CPU":       "0.5",
		"REDIS_POOL_SIZE": "50 (prod) / 20 (dev)",

		// MinIO
		// WARNING: MINIO_ROOT_USER and MINIO_ROOT_PASSWORD below are dev-only defaults.
		// Both MUST be overridden with strong unique values in staging/prod .env files.
		// nself start will exit non-zero if either is 'minioadmin' in staging/prod.
		"MINIO_ENABLED":         "false",
		"MINIO_VERSION":         "latest",
		"MINIO_PORT":            "9000",
		"MINIO_CONSOLE_PORT":    "9001",
		"MINIO_ROOT_USER":       "minioadmin",
		"MINIO_ROOT_PASSWORD":   "minioadmin",
		"MINIO_DEFAULT_BUCKETS": "uploads,public,private,temp",
		"MINIO_REGION":          "us-east-1",
		"S3_BUCKET":             "nself",
		"STORAGE_VERSION":       "0.6.1",
		"STORAGE_ROUTE":         "storage",
		"STORAGE_CONSOLE_ROUTE": "storage-console",
		"MINIO_MEMORY":          "1G",
		"MINIO_CPU":             "0.5",

		// Mailpit
		"MAILPIT_ENABLED":      "false",
		"MAILPIT_VERSION":      "latest",
		"MAILPIT_SMTP_PORT":    "1025",
		"MAILPIT_UI_PORT":      "8025",
		"MAILPIT_MAX_MESSAGES": "500",
		"MAILPIT_ROUTE":        "mail",

		// Functions
		"FUNCTIONS_ENABLED": "false",
		"FUNCTIONS_VERSION": "latest",
		"FUNCTIONS_PORT":    "3008",
		"FUNCTIONS_ROUTE":   "functions",

		// MLflow
		"MLFLOW_ENABLED":          "false",
		"MLFLOW_VERSION":          "2.9.2",
		"MLFLOW_PORT":             "5000",
		"MLFLOW_ROUTE":            "mlflow",
		"MLFLOW_DB_NAME":          "mlflow",
		"MLFLOW_ARTIFACTS_BUCKET": "mlflow-artifacts",
		"MLFLOW_AUTH_USERNAME":    "admin",

		// Admin
		"NSELF_ADMIN_ENABLED": "false",
		"NSELF_ADMIN_VERSION": "latest",
		"NSELF_ADMIN_PORT":    "3021",
		"NSELF_ADMIN_ROUTE":   "admin",

		// Search
		"SEARCH_ENABLED":                      "false",
		"SEARCH_ENGINE":                       "meilisearch",
		"SEARCH_PORT":                         "7700",
		"SEARCH_ROUTE":                        "search",
		"SEARCH_AUTO_INDEX":                   "true",
		"SEARCH_LANGUAGE":                     "en",
		"MEILISEARCH_VERSION":                 "v1.6",
		"MEILISEARCH_ENV":                     "development",
		"TYPESENSE_VERSION":                   "27.1",
		"TYPESENSE_LOG_LEVEL":                 "info",
		"TYPESENSE_NUM_MEMORY_SHARDS":         "4",
		"TYPESENSE_SNAPSHOT_INTERVAL_SECONDS": "3600",
		"TYPESENSE_ENABLE_CORS":               "true",
		"ELASTICSEARCH_VERSION":               "8.11.3",
		"ELASTICSEARCH_PORT":                  "9200",
		"ELASTICSEARCH_MEMORY":                "1Gi",
		"MEILISEARCH_WARMUP_QUERIES":          "",

		// Monitoring
		"MONITORING_ENABLED":     "false",
		"PROMETHEUS_PORT":        "9090",
		"GRAFANA_PORT":           "3000",
		"GRAFANA_ADMIN_USER":     "admin",
		"GRAFANA_ROUTE":          "grafana",
		"LOKI_PORT":              "3100",
		"TEMPO_PORT":             "3200",
		"ALERTMANAGER_PORT":      "9093",
		"CADVISOR_PORT":          "8082",
		"NODE_EXPORTER_PORT":     "9100",
		"POSTGRES_EXPORTER_PORT": "9187",
		"REDIS_EXPORTER_PORT":    "9121",

		// Email
		"EMAIL_PROVIDER": "mailpit",
		"SMTP_PORT":      "587",
		"AWS_REGION":     "us-east-1",

		// Backup
		"BACKUP_ENABLED":               "false",
		"BACKUP_DIR":                   "./backups",
		"BACKUP_SCHEDULE":              "0 2 * * *",
		"BACKUP_RETENTION_DAYS":        "30",
		"BACKUP_REMOTE":                "",
		"BACKUP_ENCRYPTION":            "false",
		"BACKUP_AGE_RECIPIENTS":        "",
		"BACKUP_SCHEDULE_FULL":         "0 3 * * *",
		"BACKUP_WAL_INTERVAL_SECONDS":  "60",
		"BACKUP_RETENTION_DAILY":       "7",
		"BACKUP_RETENTION_WEEKLY":      "4",
		"BACKUP_RETENTION_MONTHLY":     "12",
		"BACKUP_RESTORE_TEST_SCHEDULE": "0 5 * * 0",
		"BACKUP_ALERT_ON_FAILURE":      "true",
		"BACKUP_S3_ACCESS_KEY_ID":      "",
		"BACKUP_S3_SECRET_ACCESS_KEY":  "",
		"BACKUP_S3_REGION":             "",
		"BACKUP_S3_ENDPOINT":           "",
		"BACKUP_CRITICAL_TABLES":       "",

		// Disaster Recovery
		"DR_SECONDARY_REGION": "",
		"DR_STANDBY_HOST":     "",
		"DR_DRILL_SCHEDULE":   "",

		// Plugin Pro
		"NOTIFY_PORT":                 "3712",
		"CRON_PORT":                   "3713",
		"CRON_RETENTION_DAYS":         "90",
		"PLUGIN_AI_MEMORY_LIMIT":      "1g",
		"PLUGIN_AI_CPU_LIMIT":         "1.0",
		"PLUGIN_MUX_MEMORY_LIMIT":     "512m",
		"PLUGIN_MUX_CPU_LIMIT":        "0.5",
		"PLUGIN_CLAW_MEMORY_LIMIT":    "512m",
		"PLUGIN_CLAW_CPU_LIMIT":       "0.5",
		"PLUGIN_DEFAULT_MEMORY_LIMIT": "512m",
		"PLUGIN_DEFAULT_CPU_LIMIT":    "0.5",

		// Plugin System
		"NSELF_PLUGIN_DIR":         "~/.nself/plugins",
		"NSELF_PLUGIN_CACHE":       "~/.nself/cache/plugins",
		"NSELF_PLUGIN_REGISTRY":    "https://plugins.nself.org",
		"NSELF_REGISTRY_CACHE_TTL": "300",
		"NSELF_PING_API_URL":       "https://ping.nself.org",
		"NSELF_PRICING_URL":        "https://nself.org/pricing",

		// Docker
		"DOCKER_LOG_MAX_SIZE":        "10m",
		"DOCKER_LOG_MAX_FILE":        "3",
		"DOCKER_STOP_GRACE_PERIOD":   "30s",
		"NSELF_DOCKER_BUILD_TIMEOUT": "300",

		// Start/Stop
		"NSELF_START_MODE":            "smart",
		"NSELF_HEALTH_CHECK_TIMEOUT":  "120",
		"NSELF_HEALTH_CHECK_INTERVAL": "2",
		"NSELF_HEALTH_CHECK_REQUIRED": "80",
		"NSELF_CLEANUP_ON_START":      "auto",
		"NSELF_ALLOW_EXPOSED_PORTS":   "false",
		"NSELF_PARALLEL_LIMIT":        "5",
		"NSELF_LOG_LEVEL":             "info",
		"NSELF_SKIP_HEALTH_CHECKS":    "false",
		"NSELF_STOP_TIMEOUT":          "30",
	}
	return defaults[key]
}
