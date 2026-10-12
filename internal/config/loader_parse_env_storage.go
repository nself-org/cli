package config

// loader_parse_env_storage.go — env var -> Config field mapping: MinIO/Storage,
// Mailpit, Functions, MLflow, Admin, Search. Split from loader_parse_env.go
// (T-P6-E2-W1-S1-T3).
// Inputs:  os.Environ, read into the cfg passed in by parseEnvToConfig.
// Outputs: none — mutates cfg in place.
// Constraints: pure os.Getenv reads only, same rules as loader_parse_env.go.

import (
	"os"

	"github.com/nself-org/cli/internal/compat"
)

// parseEnvStorage fills the MinIO/Mailpit/Functions/MLflow/Admin/Search fields.
func parseEnvStorage(cfg *Config) {
	// ── MinIO / Storage ──────────────────────────────────────────────
	// Backward compat: STORAGE_ENABLED=true implies MINIO_ENABLED=true.
	// Gap #8: also infer intent to enable storage when the user has explicitly
	// set MinIO-specific credentials/config (MINIO_ROOT_USER/PASSWORD, their
	// MINIO_ACCESS_KEY/MINIO_SECRET_KEY aliases, or S3_ACCESS_KEY/S3_SECRET_KEY)
	// without also setting MINIO_ENABLED/STORAGE_ENABLED. This matches how apps
	// like ntask declare a full MinIO credential surface in .env.example but
	// never set the ENABLED toggle — the storage service must still generate.
	_, minioEnabledSet := os.LookupEnv("MINIO_ENABLED")
	_, storageEnabledSet := os.LookupEnv("STORAGE_ENABLED")
	minioCredentialVars := []string{
		"MINIO_ROOT_USER", "MINIO_ROOT_PASSWORD",
		"MINIO_ACCESS_KEY", "MINIO_SECRET_KEY",
	}
	coreS3KeyVars := []string{"S3_ACCESS_KEY", "S3_SECRET_KEY"}
	minioIntentVars := append(append([]string{}, minioCredentialVars...), coreS3KeyVars...)
	// compat.V15(P7-PLUG-60): S3 keys infer MinIO intent -> core object storage keys do not enable MinIO
	if compat.V15() && getEnvBool("OBJECT_STORAGE_ENABLED", false) {
		minioIntentVars = minioCredentialVars
	}
	hasMinioIntent := false
	for _, k := range minioIntentVars {
		if v, ok := os.LookupEnv(k); ok && v != "" {
			hasMinioIntent = true
			break
		}
	}
	minioEnabled := getEnvBool("MINIO_ENABLED", false) || getEnvBool("STORAGE_ENABLED", false)
	if !minioEnabled && !minioEnabledSet && !storageEnabledSet && hasMinioIntent {
		minioEnabled = true
	}

	// MINIO_ACCESS_KEY/MINIO_SECRET_KEY alias to MINIO_ROOT_USER/MINIO_ROOT_PASSWORD
	// (gap #2). The internal MinIO container always reads MINIO_ROOT_USER/
	// MINIO_ROOT_PASSWORD; apps commonly document the S3-style ACCESS/SECRET
	// key names instead. ROOT_* wins when both are set, for back-compat with
	// anyone already using ROOT_* directly.
	rootUser := os.Getenv("MINIO_ROOT_USER")
	if rootUser == "" {
		rootUser = os.Getenv("MINIO_ACCESS_KEY")
	}
	rootPassword := os.Getenv("MINIO_ROOT_PASSWORD")
	if rootPassword == "" {
		rootPassword = os.Getenv("MINIO_SECRET_KEY")
	}

	cfg.Minio = MinioConfig{
		Enabled:        minioEnabled,
		Version:        os.Getenv("MINIO_VERSION"),
		Port:           getEnvInt("MINIO_PORT", 0),
		ConsolePort:    getEnvInt("MINIO_CONSOLE_PORT", 0),
		RootUser:       rootUser,
		RootPassword:   rootPassword,
		DefaultBuckets: os.Getenv("MINIO_DEFAULT_BUCKETS"),
		Region:         os.Getenv("MINIO_REGION"),
		S3AccessKey:    os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:    os.Getenv("S3_SECRET_KEY"),
		S3Bucket:       os.Getenv("S3_BUCKET"),
		StorageVersion: os.Getenv("STORAGE_VERSION"),
		StorageRoute:   os.Getenv("STORAGE_ROUTE"),
		ConsoleRoute:   os.Getenv("STORAGE_CONSOLE_ROUTE"),
		MemLimit:       os.Getenv("MINIO_MEMORY"),
		CPULimit:       os.Getenv("MINIO_CPU"),
	}

	// ── Mailpit ──────────────────────────────────────────────────────
	cfg.Mailpit = MailpitConfig{
		Enabled:     getEnvBool("MAILPIT_ENABLED", false),
		Version:     os.Getenv("MAILPIT_VERSION"),
		SMTPPort:    getEnvInt("MAILPIT_SMTP_PORT", 0),
		UIPort:      getEnvInt("MAILPIT_UI_PORT", 0),
		MaxMessages: getEnvInt("MAILPIT_MAX_MESSAGES", 0),
		Route:       getEnvOr("MAILPIT_ROUTE", os.Getenv("MAIL_ROUTE")),
		UIUser:      getEnvOr("MAILPIT_UI_USER", "admin"),
		UIPassword:  os.Getenv("MAILPIT_UI_PASSWORD"),
	}

	// ── Functions ────────────────────────────────────────────────────
	cfg.Functions = FunctionsConfig{
		Enabled: getEnvBool("FUNCTIONS_ENABLED", false),
		Version: os.Getenv("FUNCTIONS_VERSION"),
		Port:    getEnvInt("FUNCTIONS_PORT", 0),
		Route:   os.Getenv("FUNCTIONS_ROUTE"),
	}

	// ── MLflow ───────────────────────────────────────────────────────
	cfg.MLflow = MLflowConfig{
		Enabled:         getEnvBool("MLFLOW_ENABLED", false),
		Version:         os.Getenv("MLFLOW_VERSION"),
		Port:            getEnvInt("MLFLOW_PORT", 0),
		Route:           os.Getenv("MLFLOW_ROUTE"),
		DBName:          os.Getenv("MLFLOW_DB_NAME"),
		ArtifactsBucket: os.Getenv("MLFLOW_ARTIFACTS_BUCKET"),
		AuthEnabled:     getEnvBool("MLFLOW_AUTH_ENABLED", false),
		AuthUsername:    os.Getenv("MLFLOW_AUTH_USERNAME"),
		AuthPassword:    os.Getenv("MLFLOW_AUTH_PASSWORD"),
	}

	// ── Admin ────────────────────────────────────────────────────────
	cfg.Admin = AdminConfig{
		Enabled:      getEnvBool("NSELF_ADMIN_ENABLED", false),
		Version:      os.Getenv("NSELF_ADMIN_VERSION"),
		Port:         getEnvInt("NSELF_ADMIN_PORT", 0),
		Route:        os.Getenv("NSELF_ADMIN_ROUTE"),
		DevMode:      getEnvBool("NSELF_ADMIN_DEV", false),
		DevPort:      getEnvInt("NSELF_ADMIN_DEV_PORT", 0),
		SecretKey:    os.Getenv("ADMIN_SECRET_KEY"),
		PasswordHash: os.Getenv("ADMIN_PASSWORD_HASH"),
	}

	// ── Search (provider-agnostic) ───────────────────────────────────
	cfg.Search = SearchConfig{
		Enabled:     getEnvBool("SEARCH_ENABLED", false),
		Engine:      getEnvOr("SEARCH_ENGINE", os.Getenv("SEARCH_PROVIDER")),
		Port:        getEnvInt("SEARCH_PORT", 0),
		APIKey:      os.Getenv("SEARCH_API_KEY"),
		Route:       os.Getenv("SEARCH_ROUTE"),
		IndexPrefix: os.Getenv("SEARCH_INDEX_PREFIX"),
		AutoIndex:   getEnvBool("SEARCH_AUTO_INDEX", true),
		Language:    os.Getenv("SEARCH_LANGUAGE"),
		MeiliSearch: MeiliSearchConfig{
			Version:   os.Getenv("MEILISEARCH_VERSION"),
			MasterKey: os.Getenv("MEILISEARCH_MASTER_KEY"),
			Env:       getEnvOr("MEILISEARCH_ENV", os.Getenv("MEILI_ENV")),
		},
		Typesense: TypesenseConfig{
			Version:           os.Getenv("TYPESENSE_VERSION"),
			APIKey:            os.Getenv("TYPESENSE_API_KEY"),
			EnableCORS:        getEnvBool("TYPESENSE_ENABLE_CORS", false),
			LogLevel:          os.Getenv("TYPESENSE_LOG_LEVEL"),
			NumMemoryShards:   getEnvInt("TYPESENSE_NUM_MEMORY_SHARDS", 0),
			SnapshotIntervalS: getEnvInt("TYPESENSE_SNAPSHOT_INTERVAL_SECONDS", 0),
		},
		Elasticsearch: ElasticsearchConfig{
			Version:  os.Getenv("ELASTICSEARCH_VERSION"),
			Port:     getEnvInt("ELASTICSEARCH_PORT", 0),
			Password: os.Getenv("ELASTICSEARCH_PASSWORD"),
			Memory:   os.Getenv("ELASTICSEARCH_MEMORY"),
		},
	}

}
