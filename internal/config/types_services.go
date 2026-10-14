package config

// types_services.go — config struct fields for secondary services and search/monitoring.
//
// Purpose: define the Mailpit, Functions, MLflow, Admin, Search (MeiliSearch/Typesense/Elasticsearch) and Monitoring sub-config types embedded in Config, split out of types.go for file size.
// Inputs: none (pure type declarations, populated by the loader and defaults).
// Outputs: the sub-config types used as fields on Config.
// Constraints: pure move from types.go (CLI-R12 Batch F); no behaviour change. Keep in sync with defaults_storage_services.go, which populates these fields.

// MailpitConfig holds Mailpit local email testing configuration.
type MailpitConfig struct {
	Enabled     bool   `env:"MAILPIT_ENABLED"`
	Version     string `env:"MAILPIT_VERSION"`      // latest
	SMTPPort    int    `env:"MAILPIT_SMTP_PORT"`    // 1025
	UIPort      int    `env:"MAILPIT_UI_PORT"`      // 8025
	MaxMessages int    `env:"MAILPIT_MAX_MESSAGES"` // 500
	Route       string `env:"MAILPIT_ROUTE"`        // mail.{BD}
	UIUser      string `env:"MAILPIT_UI_USER"`      // admin (default)
	UIPassword  string `env:"MAILPIT_UI_PASSWORD"`
}

// FunctionsConfig holds serverless functions runtime configuration.
type FunctionsConfig struct {
	Enabled bool   `env:"FUNCTIONS_ENABLED"`
	Version string `env:"FUNCTIONS_VERSION"` // latest
	Port    int    `env:"FUNCTIONS_PORT"`    // 3008
	Route   string `env:"FUNCTIONS_ROUTE"`   // functions.{BD}

	// Runtime reads FUNCTIONS_RUNTIME from the .env cascade and selects the
	// container image: node (default when empty), deno, or python. Load rejects
	// any other value; runtime names are case sensitive.
	Runtime string `env:"FUNCTIONS_RUNTIME"` // node|deno|python

	// Resource limits for the functions container.
	Memory  string `env:"FUNCTIONS_MEMORY"`  // 256M
	CPU     string `env:"FUNCTIONS_CPU"`     // 0.5
	Timeout string `env:"FUNCTIONS_TIMEOUT"` // 30s
}

// MLflowConfig holds MLflow experiment tracking configuration.
// Compose generation is plugin-managed: nself plugin install mlflow
// Enabled, Route, and Port are read by ssl/domains.go, urls.go, and doctor.go.
// All other fields are consumed exclusively by the nself-mlflow plugin at install time.
type MLflowConfig struct {
	Enabled         bool   `env:"MLFLOW_ENABLED"`
	Route           string `env:"MLFLOW_ROUTE"`            // mlflow.{BD} — read by ssl/domains.go, urls.go, doctor.go
	Version         string `env:"MLFLOW_VERSION"`          // plugin-managed: populated by nself plugin install mlflow
	Port            int    `env:"MLFLOW_PORT"`             // read by doctor.go for port-conflict checks; plugin-managed: populated by nself plugin install mlflow
	DBName          string `env:"MLFLOW_DB_NAME"`          // plugin-managed: populated by nself plugin install mlflow
	ArtifactsBucket string `env:"MLFLOW_ARTIFACTS_BUCKET"` // plugin-managed: populated by nself plugin install mlflow
	AuthEnabled     bool   `env:"MLFLOW_AUTH_ENABLED"`     // plugin-managed: populated by nself plugin install mlflow
	AuthUsername    string `env:"MLFLOW_AUTH_USERNAME"`    // plugin-managed: populated by nself plugin install mlflow
	AuthPassword    string `env:"MLFLOW_AUTH_PASSWORD"`    // plugin-managed: populated by nself plugin install mlflow
}

// AdminConfig holds nSelf Admin GUI configuration.
type AdminConfig struct {
	Enabled      bool   `env:"NSELF_ADMIN_ENABLED"`
	Version      string `env:"NSELF_ADMIN_VERSION"`  // latest
	Port         int    `env:"NSELF_ADMIN_PORT"`     // 3021
	Route        string `env:"NSELF_ADMIN_ROUTE"`    // admin.{BD}
	DevMode      bool   `env:"NSELF_ADMIN_DEV"`      // false
	DevPort      int    `env:"NSELF_ADMIN_DEV_PORT"` // 3000
	SecretKey    string `env:"ADMIN_SECRET_KEY"`
	PasswordHash string `env:"ADMIN_PASSWORD_HASH"`
}

// SearchConfig holds search engine configuration (provider-agnostic).
type SearchConfig struct {
	Enabled     bool   `env:"SEARCH_ENABLED"`
	Engine      string `env:"SEARCH_ENGINE"`  // meilisearch, typesense, etc.
	Port        int    `env:"SEARCH_PORT"`    // auto from provider
	APIKey      string `env:"SEARCH_API_KEY"` // auto-generated if unset
	Route       string `env:"SEARCH_ROUTE"`   // search.{BD}
	IndexPrefix string `env:"SEARCH_INDEX_PREFIX"`
	AutoIndex   bool   `env:"SEARCH_AUTO_INDEX"` // true
	Language    string `env:"SEARCH_LANGUAGE"`   // en

	// Provider-specific (only populated for active provider)
	MeiliSearch   MeiliSearchConfig
	Typesense     TypesenseConfig
	Elasticsearch ElasticsearchConfig
}

// MeiliSearchConfig holds MeiliSearch-specific configuration.
type MeiliSearchConfig struct {
	Version   string `env:"MEILISEARCH_VERSION"` // v1.6
	MasterKey string `env:"MEILISEARCH_MASTER_KEY"`
	Env       string `env:"MEILISEARCH_ENV"` // development
}

// TypesenseConfig holds Typesense-specific configuration.
type TypesenseConfig struct {
	Version           string `env:"TYPESENSE_VERSION"` // 27.1
	APIKey            string `env:"TYPESENSE_API_KEY"`
	EnableCORS        bool   `env:"TYPESENSE_ENABLE_CORS"`
	LogLevel          string `env:"TYPESENSE_LOG_LEVEL"`
	NumMemoryShards   int    `env:"TYPESENSE_NUM_MEMORY_SHARDS"`
	SnapshotIntervalS int    `env:"TYPESENSE_SNAPSHOT_INTERVAL_SECONDS"`
}

// ElasticsearchConfig holds Elasticsearch-specific configuration.
type ElasticsearchConfig struct {
	Version  string `env:"ELASTICSEARCH_VERSION"` // 8.11.3
	Port     int    `env:"ELASTICSEARCH_PORT"`    // 9200
	Password string `env:"ELASTICSEARCH_PASSWORD"`
	Memory   string `env:"ELASTICSEARCH_MEMORY"` // 1Gi
}

// MonitoringConfig holds monitoring stack configuration.
// Compose generation is plugin-managed: nself plugin install monitoring
// Enabled, GrafanaEnabled, GrafanaRoute, GrafanaAdminPassword, and GrafanaPort are read by
// ssl/domains.go (SSL SANs), config/validator.go (password check), urls.go, and doctor.go.
// All other fields are consumed exclusively by the nself-monitoring plugin at install time.
type MonitoringConfig struct {
	Enabled              bool   `env:"MONITORING_ENABLED"`
	GrafanaEnabled       bool   `env:"GRAFANA_ENABLED"`
	GrafanaRoute         string `env:"GRAFANA_ROUTE"`             // read by ssl/domains.go, urls.go, doctor.go
	GrafanaAdminPassword string `env:"GRAFANA_ADMIN_PASSWORD"`    // read by config/validator.go
	PrometheusEnabled    bool   `env:"PROMETHEUS_ENABLED"`        // plugin-managed: populated by nself plugin install monitoring
	PrometheusPort       int    `env:"PROMETHEUS_PORT"`           // plugin-managed: populated by nself plugin install monitoring
	GrafanaPort          int    `env:"GRAFANA_PORT"`              // read by urls.go and doctor.go for port display; plugin-managed: populated by nself plugin install monitoring
	GrafanaAdminUser     string `env:"GRAFANA_ADMIN_USER"`        // plugin-managed: populated by nself plugin install monitoring
	LokiEnabled          bool   `env:"LOKI_ENABLED"`              // plugin-managed: populated by nself plugin install monitoring
	LokiPort             int    `env:"LOKI_PORT"`                 // plugin-managed: populated by nself plugin install monitoring
	PromtailEnabled      bool   `env:"PROMTAIL_ENABLED"`          // plugin-managed: populated by nself plugin install monitoring
	TempoEnabled         bool   `env:"TEMPO_ENABLED"`             // plugin-managed: populated by nself plugin install monitoring
	TempoPort            int    `env:"TEMPO_PORT"`                // plugin-managed: populated by nself plugin install monitoring
	AlertmanagerEnabled  bool   `env:"ALERTMANAGER_ENABLED"`      // plugin-managed: populated by nself plugin install monitoring
	AlertmanagerPort     int    `env:"ALERTMANAGER_PORT"`         // plugin-managed: populated by nself plugin install monitoring
	CadvisorEnabled      bool   `env:"CADVISOR_ENABLED"`          // plugin-managed: populated by nself plugin install monitoring
	CadvisorPort         int    `env:"CADVISOR_PORT"`             // plugin-managed: populated by nself plugin install monitoring
	NodeExporterEnabled  bool   `env:"NODE_EXPORTER_ENABLED"`     // plugin-managed: populated by nself plugin install monitoring
	NodeExporterPort     int    `env:"NODE_EXPORTER_PORT"`        // plugin-managed: populated by nself plugin install monitoring
	PGExporterEnabled    bool   `env:"POSTGRES_EXPORTER_ENABLED"` // plugin-managed: populated by nself plugin install monitoring
	PGExporterPort       int    `env:"POSTGRES_EXPORTER_PORT"`    // plugin-managed: populated by nself plugin install monitoring
	RedisExporterEnabled bool   `env:"REDIS_EXPORTER_ENABLED"`    // plugin-managed: populated by nself plugin install monitoring
	RedisExporterPort    int    `env:"REDIS_EXPORTER_PORT"`       // plugin-managed: populated by nself plugin install monitoring

	// S34 additions
	PrometheusRetention      string `env:"PROMETHEUS_RETENTION"`       // e.g. "30d"
	LokiHotDays              int    `env:"LOKI_HOT_DAYS"`              // default 30
	LokiColdDays             int    `env:"LOKI_COLD_DAYS"`             // default 365
	AlertmanagerPagerdutyKey string `env:"ALERTMANAGER_PAGERDUTY_KEY"` // PagerDuty integration key

	// Watchdog
	WatchdogEnabled                bool   `env:"WATCHDOG_ENABLED"`
	WatchdogCircuitBreakerAttempts int    `env:"WATCHDOG_CIRCUIT_BREAKER_ATTEMPTS"` // default 3
	WatchdogCircuitBreakerWindow   string `env:"WATCHDOG_CIRCUIT_BREAKER_WINDOW"`   // default 10m
	WatchdogEscalationWebhook      string `env:"WATCHDOG_ESCALATION_WEBHOOK"`

	// Queue/Jobs
	QueueWorkersPerQueue   int `env:"QUEUE_WORKERS_PER_QUEUE"`   // default 2
	QueueDLQAlertThreshold int `env:"QUEUE_DLQ_ALERT_THRESHOLD"` // default 100

	// Promotion
	PromoteRequiresTwoApprovers bool `env:"PROMOTE_REQUIRES_TWO_APPROVERS"`
}
