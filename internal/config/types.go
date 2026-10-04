package config

// Config is the top-level configuration struct for an nSelf project.
// All fields are populated from the .env cascade and environment variables.
type Config struct {
	// Core
	ProjectName        string `env:"PROJECT_NAME"`
	BaseDomain         string `env:"BASE_DOMAIN"`
	Env                string `env:"ENV"` // dev, staging, prod
	ProjectDescription string `env:"PROJECT_DESCRIPTION"`
	AdminEmail         string `env:"ADMIN_EMAIL"`
	DBEnvSeeds         bool   `env:"DB_ENV_SEEDS"`
	// AppName is an OPTIONAL app-prefix used to namespace generated core
	// nginx subdomains (gap #5): when set, routes render as
	// "api.{AppName}.{BASE_DOMAIN}" / "auth.{AppName}.{BASE_DOMAIN}" instead
	// of the bare "api.{BASE_DOMAIN}" / "auth.{BASE_DOMAIN}" scheme. Deliberately
	// distinct from PROJECT_NAME (which is always set, for container/network
	// naming) so existing single-app deployments (ummat, unity) that only set
	// PROJECT_NAME are unaffected — AppName defaults to "" (bare scheme).
	AppName string `env:"APP_NAME"`

	// PostgreSQL
	Postgres PostgresConfig

	// Hasura
	Hasura HasuraConfig

	// Auth
	Auth AuthConfig

	// Nginx
	Nginx NginxConfig

	// SSL
	SSLMode           string `env:"SSL_MODE"`            // local, letsencrypt, custom, none
	SSLProvider       string `env:"SSL_PROVIDER"`        // cloudflare, route53, digitalocean, custom
	SSLWildcardDomain string `env:"SSL_WILDCARD_DOMAIN"` // *.example.com
	ExtraSSLDomains   string `env:"EXTRA_SSL_DOMAINS"`   // comma-separated
	CloudflareAPIKey  string `env:"CLOUDFLARE_API_KEY"`  // DNS-01 challenge

	// WAF
	WAFMode string `env:"WAF_MODE"` // off, detection, blocking

	// Optional Services
	Redis      RedisConfig
	Minio      MinioConfig
	Mailpit    MailpitConfig
	Functions  FunctionsConfig
	MLflow     MLflowConfig
	Admin      AdminConfig
	Monitoring MonitoringConfig

	// Search (provider-agnostic)
	Search SearchConfig

	// Email Provider
	Email EmailConfig

	// PgBouncer connection pooler
	PgBouncer PgBouncerConfig

	// Backup & Recovery
	Backup BackupConfig

	// Disaster Recovery
	DR DRConfig

	// Multi-Tenancy & Billing
	Tenant TenantConfig

	// License
	License LicenseConfig

	// Secrets Management
	Secrets SecretsConfig

	// Plugin Pro Configuration
	PluginConfig PluginProConfig

	// Plugin System
	PluginSystem PluginSystemConfig

	// API Docs (Scalar)
	ApiDocs ApiDocsConfig

	// Custom Services
	CustomServices []CustomService // CS_1..CS_10

	// Frontend Apps
	FrontendApps []FrontendApp // FRONTEND_APP_1..FRONTEND_APP_20

	// Remote Schemas
	RemoteSchemas []RemoteSchema

	// Internal Routes (up to 20)
	InternalRoutes []InternalRoute

	// Docker
	DockerNetwork      string `env:"DOCKER_NETWORK"`
	DockerLogMaxSize   string `env:"DOCKER_LOG_MAX_SIZE"`        // 10m
	DockerLogMaxFile   string `env:"DOCKER_LOG_MAX_FILE"`        // 3
	DockerStopGrace    string `env:"DOCKER_STOP_GRACE_PERIOD"`   // 30s
	DockerBuildTimeout int    `env:"NSELF_DOCKER_BUILD_TIMEOUT"` // 300

	// Start/Stop behavior
	StartMode           string `env:"NSELF_START_MODE"`           // smart, fresh, force
	HealthCheckTimeout  int    `env:"NSELF_HEALTH_CHECK_TIMEOUT"` // seconds
	HealthCheckInterval int    `env:"NSELF_HEALTH_CHECK_INTERVAL"`
	HealthCheckRequired int    `env:"NSELF_HEALTH_CHECK_REQUIRED"` // percentage
	CleanupOnStart      string `env:"NSELF_CLEANUP_ON_START"`      // auto/always/never
	AllowExposedPorts   bool   `env:"NSELF_ALLOW_EXPOSED_PORTS"`
	ParallelLimit       int    `env:"NSELF_PARALLEL_LIMIT"` // 5
	LogLevel            string `env:"NSELF_LOG_LEVEL"`      // info
	SkipHealthChecks    bool   `env:"NSELF_SKIP_HEALTH_CHECKS"`
	StopTimeout         int    `env:"NSELF_STOP_TIMEOUT"` // 30

	// Federation — GraphQL Federation via Apollo Router (G05).
	// When true, nself build injects Apollo Router (CS_7) and composes a
	// supergraph schema from installed plugin subgraphs. Default: false.
	FederationEnabled bool `env:"NSELF_FEDERATION"`

	// EmbeddedPG — when true, nself build omits the Docker postgres service
	// and instead relies on the pglite/wasmtime embedded runtime started by
	// `nself start --embedded-pg`. Hasura is wired via a Unix-domain socket
	// bridge. Default: false.
	EmbeddedPG bool `env:"NSELF_EMBEDDED_PG"`

	// Passthrough: arbitrary env vars matching patterns (AUTH_PROVIDER_*, REMOTE_SCHEMA_*, etc.)
	Passthrough map[string]string
}

// IsProduction reports whether the project environment is production.
// Both "prod" and "production" are treated as production; the loader normalises
// "production" → "prod" via normalizeEnv, so only "prod" is checked here.
func (c *Config) IsProduction() bool {
	return c.Env == "prod" || c.Env == "production"
}
