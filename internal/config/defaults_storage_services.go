package config

// defaults_storage_services.go — default value application for Redis, MinIO and secondary services.
//
// Purpose: Fill in unset Redis, MinIO, Mailpit, Functions, MLflow, Admin and Search fields on a loaded Config with the CLI's standard defaults, split out of defaults.go for file size.
// Inputs: a *Config already populated by the loader.
// Outputs: the same *Config with these service fields defaulted in place.
// Constraints: pure move from defaults.go (CLI-R12 Batch F); no behaviour change. Keep in sync with ApplyDefaults in defaults.go, which calls these in order.

import (
	"fmt"
	"log/slog"
)

// applyDefaultsRedis sets Redis connection, memory, and pool defaults.
func applyDefaultsRedis(cfg *Config) {
	if cfg.Redis.Version == "" {
		cfg.Redis.Version = "7-alpine"
		slog.Debug("default", "key", "REDIS_VERSION", "value", cfg.Redis.Version)
	}
	if cfg.Redis.Port == 0 {
		cfg.Redis.Port = 6379
		slog.Debug("default", "key", "REDIS_PORT", "value", fmt.Sprintf("%d", cfg.Redis.Port))
	}
	if cfg.Redis.Memory == "" {
		cfg.Redis.Memory = "512M"
		slog.Debug("default", "key", "REDIS_MEMORY", "value", cfg.Redis.Memory)
	}
	if cfg.Redis.CPU == "" {
		cfg.Redis.CPU = "0.5"
		slog.Debug("default", "key", "REDIS_CPU", "value", cfg.Redis.CPU)
	}
	if cfg.Redis.PoolSize == 0 {
		if cfg.Env == "dev" {
			cfg.Redis.PoolSize = 20
		} else {
			cfg.Redis.PoolSize = 50
		}
		slog.Debug("default", "key", "REDIS_POOL_SIZE", "value", fmt.Sprintf("%d", cfg.Redis.PoolSize))
	}
}

// applyDefaultsMinio sets MinIO object-storage defaults by delegating to credential and storage helpers.
func applyDefaultsMinio(cfg *Config) error {
	applyDefaultsMinioConn(cfg)
	// Guard: reject minioadmin defaults in staging/prod before any container is started.
	if err := ValidateMinioCredentials(cfg); err != nil {
		return err
	}
	applyDefaultsMinioStorage(cfg)
	return nil
}

// applyDefaultsMinioConn sets MinIO port, root credentials, and resource limits.
func applyDefaultsMinioConn(cfg *Config) {
	if cfg.Minio.Version == "" {
		cfg.Minio.Version = "latest"
		slog.Debug("default", "key", "MINIO_VERSION", "value", cfg.Minio.Version)
	}
	if cfg.Minio.Port == 0 {
		cfg.Minio.Port = 9000
		slog.Debug("default", "key", "MINIO_PORT", "value", fmt.Sprintf("%d", cfg.Minio.Port))
	}
	if cfg.Minio.ConsolePort == 0 {
		cfg.Minio.ConsolePort = 9001
		slog.Debug("default", "key", "MINIO_CONSOLE_PORT", "value", fmt.Sprintf("%d", cfg.Minio.ConsolePort))
	}
	if cfg.Minio.RootUser == "" {
		cfg.Minio.RootUser = "minioadmin"
		slog.Debug("default", "key", "MINIO_ROOT_USER", "value", cfg.Minio.RootUser)
	}
	if cfg.Minio.RootPassword == "" {
		cfg.Minio.RootPassword = "minioadmin"
		slog.Debug("default", "key", "MINIO_ROOT_PASSWORD", "value", "[default]")
	}
	if cfg.Minio.MemLimit == "" {
		cfg.Minio.MemLimit = "1G"
		slog.Debug("default", "key", "MINIO_MEM_LIMIT", "value", cfg.Minio.MemLimit)
	}
	if cfg.Minio.CPULimit == "" {
		cfg.Minio.CPULimit = "0.5"
		slog.Debug("default", "key", "MINIO_CPU_LIMIT", "value", cfg.Minio.CPULimit)
	}
}

// applyDefaultsMinioStorage sets MinIO bucket, region, and route defaults.
func applyDefaultsMinioStorage(cfg *Config) {
	if cfg.Minio.DefaultBuckets == "" {
		cfg.Minio.DefaultBuckets = "uploads,public,private,temp"
		slog.Debug("default", "key", "MINIO_DEFAULT_BUCKETS", "value", cfg.Minio.DefaultBuckets)
	}
	if cfg.Minio.Region == "" {
		cfg.Minio.Region = "us-east-1"
		slog.Debug("default", "key", "MINIO_REGION", "value", cfg.Minio.Region)
	}
	if cfg.Minio.S3Bucket == "" {
		cfg.Minio.S3Bucket = "nself"
		slog.Debug("default", "key", "MINIO_S3_BUCKET", "value", cfg.Minio.S3Bucket)
	}
	if cfg.Minio.StorageVersion == "" {
		cfg.Minio.StorageVersion = "0.6.1"
		slog.Debug("default", "key", "STORAGE_VERSION", "value", cfg.Minio.StorageVersion)
	}
	if cfg.Minio.StorageRoute == "" {
		cfg.Minio.StorageRoute = "storage"
		slog.Debug("default", "key", "STORAGE_ROUTE", "value", cfg.Minio.StorageRoute)
	}
	if cfg.Minio.ConsoleRoute == "" {
		cfg.Minio.ConsoleRoute = "storage-console"
		slog.Debug("default", "key", "MINIO_CONSOLE_ROUTE", "value", cfg.Minio.ConsoleRoute)
	}
}

// applyDefaultsMailpit sets Mailpit dev-email server port and UI defaults.
func applyDefaultsMailpit(cfg *Config) {
	if cfg.Mailpit.Version == "" {
		cfg.Mailpit.Version = "latest"
		slog.Debug("default", "key", "MAILPIT_VERSION", "value", cfg.Mailpit.Version)
	}
	if cfg.Mailpit.SMTPPort == 0 {
		cfg.Mailpit.SMTPPort = 1025
		slog.Debug("default", "key", "MAILPIT_SMTP_PORT", "value", fmt.Sprintf("%d", cfg.Mailpit.SMTPPort))
	}
	if cfg.Mailpit.UIPort == 0 {
		cfg.Mailpit.UIPort = 8025
		slog.Debug("default", "key", "MAILPIT_UI_PORT", "value", fmt.Sprintf("%d", cfg.Mailpit.UIPort))
	}
	if cfg.Mailpit.MaxMessages == 0 {
		cfg.Mailpit.MaxMessages = 500
		slog.Debug("default", "key", "MAILPIT_MAX_MESSAGES", "value", fmt.Sprintf("%d", cfg.Mailpit.MaxMessages))
	}
	if cfg.Mailpit.Route == "" {
		cfg.Mailpit.Route = "mail"
		slog.Debug("default", "key", "MAILPIT_ROUTE", "value", cfg.Mailpit.Route)
	}
}

// applyDefaultsFunctions sets edge-functions service port and route defaults.
func applyDefaultsFunctions(cfg *Config) {
	if cfg.Functions.Version == "" {
		cfg.Functions.Version = "latest"
		slog.Debug("default", "key", "FUNCTIONS_VERSION", "value", cfg.Functions.Version)
	}
	if cfg.Functions.Port == 0 {
		cfg.Functions.Port = 3008
		slog.Debug("default", "key", "FUNCTIONS_PORT", "value", fmt.Sprintf("%d", cfg.Functions.Port))
	}
	if cfg.Functions.Route == "" {
		cfg.Functions.Route = "functions"
		slog.Debug("default", "key", "FUNCTIONS_ROUTE", "value", cfg.Functions.Route)
	}
}

// applyDefaultsMLflow sets MLflow ML-tracking service defaults.
func applyDefaultsMLflow(cfg *Config) {
	if cfg.MLflow.Version == "" {
		cfg.MLflow.Version = "2.9.2"
		slog.Debug("default", "key", "MLFLOW_VERSION", "value", cfg.MLflow.Version)
	}
	if cfg.MLflow.Port == 0 {
		cfg.MLflow.Port = 5000
		slog.Debug("default", "key", "MLFLOW_PORT", "value", fmt.Sprintf("%d", cfg.MLflow.Port))
	}
	if cfg.MLflow.Route == "" {
		cfg.MLflow.Route = "mlflow"
		slog.Debug("default", "key", "MLFLOW_ROUTE", "value", cfg.MLflow.Route)
	}
	if cfg.MLflow.DBName == "" {
		cfg.MLflow.DBName = "mlflow"
		slog.Debug("default", "key", "MLFLOW_DB_NAME", "value", cfg.MLflow.DBName)
	}
	if cfg.MLflow.ArtifactsBucket == "" {
		cfg.MLflow.ArtifactsBucket = "mlflow-artifacts"
		slog.Debug("default", "key", "MLFLOW_ARTIFACTS_BUCKET", "value", cfg.MLflow.ArtifactsBucket)
	}
	if cfg.MLflow.AuthUsername == "" {
		cfg.MLflow.AuthUsername = "admin"
		slog.Debug("default", "key", "MLFLOW_AUTH_USERNAME", "value", cfg.MLflow.AuthUsername)
	}
}

// applyDefaultsAdmin sets the admin UI port and route defaults.
func applyDefaultsAdmin(cfg *Config) {
	if cfg.Admin.Version == "" {
		cfg.Admin.Version = "latest"
		slog.Debug("default", "key", "ADMIN_VERSION", "value", cfg.Admin.Version)
	}
	if cfg.Admin.Port == 0 {
		cfg.Admin.Port = 3021
		slog.Debug("default", "key", "ADMIN_PORT", "value", fmt.Sprintf("%d", cfg.Admin.Port))
	}
	if cfg.Admin.Route == "" {
		cfg.Admin.Route = "admin"
		slog.Debug("default", "key", "ADMIN_ROUTE", "value", cfg.Admin.Route)
	}
}

// applyDefaultsSearch sets search-engine defaults for MeiliSearch, Typesense, and Elasticsearch.
func applyDefaultsSearch(cfg *Config) {
	applyDefaultsSearchCore(cfg)
	applyDefaultsSearchEngines(cfg)
}

// applyDefaultsSearchCore sets top-level search engine, port, and language defaults.
func applyDefaultsSearchCore(cfg *Config) {
	if cfg.Search.Engine == "" {
		cfg.Search.Engine = "meilisearch"
		slog.Debug("default", "key", "SEARCH_ENGINE", "value", cfg.Search.Engine)
	}
	if cfg.Search.Port == 0 {
		cfg.Search.Port = 7700
		slog.Debug("default", "key", "SEARCH_PORT", "value", "7700")
	}
	if cfg.Search.Route == "" {
		cfg.Search.Route = "search"
		slog.Debug("default", "key", "SEARCH_ROUTE", "value", cfg.Search.Route)
	}
	if cfg.Search.Language == "" {
		cfg.Search.Language = "en"
		slog.Debug("default", "key", "SEARCH_LANGUAGE", "value", cfg.Search.Language)
	}
}

// applyDefaultsSearchEngines sets per-engine defaults (MeiliSearch, Typesense, Elasticsearch).
func applyDefaultsSearchEngines(cfg *Config) {
	if cfg.Search.MeiliSearch.Version == "" {
		cfg.Search.MeiliSearch.Version = "v1.6"
		slog.Debug("default", "key", "MEILISEARCH_VERSION", "value", cfg.Search.MeiliSearch.Version)
	}
	if cfg.Search.MeiliSearch.Env == "" {
		cfg.Search.MeiliSearch.Env = "development"
		slog.Debug("default", "key", "MEILISEARCH_ENV", "value", cfg.Search.MeiliSearch.Env)
	}
	if cfg.Search.Typesense.Version == "" {
		cfg.Search.Typesense.Version = "27.1"
		slog.Debug("default", "key", "TYPESENSE_VERSION", "value", cfg.Search.Typesense.Version)
	}
	if cfg.Search.Typesense.LogLevel == "" {
		cfg.Search.Typesense.LogLevel = "info"
		slog.Debug("default", "key", "TYPESENSE_LOG_LEVEL", "value", cfg.Search.Typesense.LogLevel)
	}
	if cfg.Search.Typesense.NumMemoryShards == 0 {
		cfg.Search.Typesense.NumMemoryShards = 4
		slog.Debug("default", "key", "TYPESENSE_NUM_MEMORY_SHARDS", "value", fmt.Sprintf("%d", cfg.Search.Typesense.NumMemoryShards))
	}
	if cfg.Search.Typesense.SnapshotIntervalS == 0 {
		cfg.Search.Typesense.SnapshotIntervalS = 3600
		slog.Debug("default", "key", "TYPESENSE_SNAPSHOT_INTERVAL_SECONDS", "value", fmt.Sprintf("%d", cfg.Search.Typesense.SnapshotIntervalS))
	}
	if !cfg.Search.Typesense.EnableCORS {
		cfg.Search.Typesense.EnableCORS = true
	}
	if cfg.Search.Elasticsearch.Version == "" {
		cfg.Search.Elasticsearch.Version = "8.11.3"
		slog.Debug("default", "key", "ELASTICSEARCH_VERSION", "value", cfg.Search.Elasticsearch.Version)
	}
	if cfg.Search.Elasticsearch.Port == 0 {
		cfg.Search.Elasticsearch.Port = 9200
		slog.Debug("default", "key", "ELASTICSEARCH_PORT", "value", fmt.Sprintf("%d", cfg.Search.Elasticsearch.Port))
	}
	if cfg.Search.Elasticsearch.Memory == "" {
		cfg.Search.Elasticsearch.Memory = "1Gi"
		slog.Debug("default", "key", "ELASTICSEARCH_MEMORY", "value", cfg.Search.Elasticsearch.Memory)
	}
}
