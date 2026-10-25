package config

// types_datastores.go — config struct fields for the primary datastores.
//
// Purpose: define the Postgres, PgBouncer, Hasura, Auth, Nginx, Redis and MinIO sub-config types embedded in Config, split out of types.go for file size.
// Inputs: none (pure type declarations, populated by the loader and defaults).
// Outputs: the sub-config types used as fields on Config.
// Constraints: pure move from types.go (CLI-R12 Batch F); no behaviour change. Keep in sync with loader_known_vars.go and defaults_postgres_hasura.go / defaults_auth_nginx.go, which populate these fields.

// PostgresConfig holds PostgreSQL database configuration.
type PostgresConfig struct {
	Version string `env:"POSTGRES_VERSION"` // 16-alpine
	// Image, when set, is the exact postgres image nself build emits and wins
	// over every other resolution (see compose.ResolvePostgresImage). Set this
	// to pin a running image (e.g. pgvector/pgvector:pg16) across regens
	// without relying on POSTGRES_EXTENSIONS inference (cli#384).
	Image string `env:"POSTGRES_IMAGE"`
	// DataVolume, when set, is the docker volume the postgres service keeps its
	// data in (compose volumes.postgres_data.name). `nself db image switch`
	// writes it; unset keeps the compose project's own postgres_data volume.
	DataVolume string   `env:"POSTGRES_DATA_VOLUME"`
	Host       string   `env:"POSTGRES_HOST"` // postgres (container name)
	Port       int      `env:"POSTGRES_PORT"` // 5432
	DB         string   `env:"POSTGRES_DB"`   // nself
	User       string   `env:"POSTGRES_USER"` // postgres
	Password   string   `env:"POSTGRES_PASSWORD"`
	Extensions []string `env:"POSTGRES_EXTENSIONS"`  // comma-separated list
	ExposePort string   `env:"POSTGRES_EXPOSE_PORT"` // auto, true, false
	MemLimit   string   `env:"POSTGRES_MEM_LIMIT"`   // 2g
	CPULimit   string   `env:"POSTGRES_CPU_LIMIT"`   // 2.0
	// MaxConnections overrides Postgres max_connections. The stock default of
	// 100 exhausts under a multi-service stack (PERF-POOL-01); nself raises it.
	MaxConnections int `env:"POSTGRES_MAX_CONNECTIONS"` // default 200
}

// PgBouncerConfig holds connection pooler configuration.
type PgBouncerConfig struct {
	Enabled           bool   `env:"PGBOUNCER_ENABLED"`
	Port              int    `env:"PGBOUNCER_PORT"`                // 6432
	PoolMode          string `env:"PGBOUNCER_POOL_MODE"`           // session, transaction, statement
	MaxClientConn     int    `env:"PGBOUNCER_MAX_CLIENT_CONN"`     // 100
	DefaultPoolSize   int    `env:"PGBOUNCER_DEFAULT_POOL_SIZE"`   // 25
	MinPoolSize       int    `env:"PGBOUNCER_MIN_POOL_SIZE"`       // 5
	ReservePoolSize   int    `env:"PGBOUNCER_RESERVE_POOL_SIZE"`   // 5
	ServerIdleTimeout int    `env:"PGBOUNCER_SERVER_IDLE_TIMEOUT"` // 600
	LogConnections    bool   `env:"PGBOUNCER_LOG_CONNECTIONS"`     // false
	LogDisconnections bool   `env:"PGBOUNCER_LOG_DISCONNECTIONS"`  // false
	AdminUsers        string `env:"PGBOUNCER_ADMIN_USERS"`         // postgres
	StatsUsers        string `env:"PGBOUNCER_STATS_USERS"`         // postgres
}

// HasuraConfig holds Hasura GraphQL engine configuration.
type HasuraConfig struct {
	Version     string `env:"HASURA_VERSION"`
	AdminSecret string `env:"HASURA_GRAPHQL_ADMIN_SECRET"`
	JWTKey      string `env:"HASURA_JWT_KEY"`
	JWTType     string `env:"HASURA_JWT_TYPE"` // HS256
	Console     bool   `env:"HASURA_GRAPHQL_ENABLE_CONSOLE"`
	DevMode     bool   `env:"HASURA_GRAPHQL_DEV_MODE"`
	CORSDomain  string `env:"HASURA_GRAPHQL_CORS_DOMAIN"`
	Route       string `env:"HASURA_ROUTE"` // api.{BASE_DOMAIN}
	Port        int    `env:"HASURA_PORT"`  // 8080
	MemLimit    string `env:"HASURA_MEM_LIMIT"`
	CPULimit    string `env:"HASURA_CPU_LIMIT"`
	LogLevel    string `env:"HASURA_GRAPHQL_LOG_LEVEL"` // warn
}

// AuthConfig holds authentication service configuration.
type AuthConfig struct {
	Version            string `env:"AUTH_VERSION"` // 0.36.0
	Port               int    `env:"AUTH_PORT"`    // 4000
	ClientURL          string `env:"AUTH_CLIENT_URL"`
	AccessTokenExpiry  int    `env:"AUTH_ACCESS_TOKEN_EXPIRES_IN"`  // seconds
	RefreshTokenExpiry int    `env:"AUTH_REFRESH_TOKEN_EXPIRES_IN"` // seconds
	Route              string `env:"AUTH_ROUTE"`
	SMTPHost           string `env:"AUTH_SMTP_HOST"`
	SMTPPort           int    `env:"AUTH_SMTP_PORT"`
	SMTPUser           string `env:"AUTH_SMTP_USER"`
	SMTPPass           string `env:"AUTH_SMTP_PASS"`
	SMTPSecure         bool   `env:"AUTH_SMTP_SECURE"`
	SMTPSender         string `env:"AUTH_SMTP_SENDER"`
	MemLimit           string `env:"AUTH_MEM_LIMIT"`           // 256m
	CPULimit           string `env:"AUTH_CPU_LIMIT"`           // 0.25
	ExtraRedirectURLs  string `env:"AUTH_EXTRA_REDIRECT_URLS"` // comma-separated extra redirect URLs
	WebAuthnEnabled    bool   `env:"AUTH_WEBAUTHN_ENABLED"`
	MFATOTPEnabled     bool   `env:"AUTH_MFA_TOTP_ENABLED"` // false
	LogLevel           string `env:"AUTH_LOG_LEVEL"`        // info
}

// NginxConfig holds Nginx reverse proxy configuration.
type NginxConfig struct {
	Version       string `env:"NGINX_VERSION"`              // alpine
	HTTPPort      int    `env:"NGINX_HTTP_PORT"`            // 80
	SSLPort       int    `env:"NGINX_HTTPS_PORT"`           // 443
	MaxBody       string `env:"NGINX_CLIENT_MAX_BODY_SIZE"` // 100M
	BindIP        string `env:"NGINX_BIND_IP"`              // computed: 127.0.0.1 (dev) or 0.0.0.0 (prod) — overridable
	AuthRateLimit string `env:"AUTH_RATE_LIMIT"`            // 30r/m
	RateLimitAPI  string `env:"RATE_LIMIT_API_RPS"`         // 30
	RateLimitAuth string `env:"RATE_LIMIT_AUTH_RPS"`        // 5
	RateLimitAI   string `env:"RATE_LIMIT_AI_RPS"`          // 10

	// FrontedBy names the stack whose nginx fronts this project's domains
	// (e.g. "nself-web"), when this project has no ingress nginx of its own.
	// Empty (the default) means this stack runs its own nginx, unchanged
	// from every prior release.
	//
	// When set, `nself build` omits the nginx service from
	// docker-compose.yml entirely and excludes it from the service counts
	// `nself status`/`nself start` expect. Without this, a stack meant to
	// sit behind another stack's nginx still generates and tries to start
	// its own nginx container, which fails to bind 80/443 (already held by
	// the fronting stack's nginx via docker-proxy) and then sits forever as
	// one unhealthy service nself status can never clear ("6/7" on ntask's
	// staging deploy, 2026-09-03, fronted by nself-web).
	//
	// This flag only removes the container that can never run. It does NOT
	// wire this project's containers onto the fronting stack's Docker
	// network — the fronting nginx's `resolver 127.0.0.11` can only reach
	// container names on a network it is attached to, so making
	// auth.task.stack-two.nself.org resolve ntask_auth still requires the
	// operator to attach the two projects' networks (or declare one as
	// external in both compose files) by hand. That is a bigger, riskier
	// change — automatically rewriting a Docker network topology from a
	// per-project flag — and belongs in a follow-up once the desired shape
	// of that wiring is decided.
	FrontedBy string `env:"NGINX_FRONTED_BY"`

	// Mode is NGINX_MODE: "" (this stack's nginx serves this project only) or
	// "shared" (this project owns a host nginx that fronted projects nested
	// under it register with, contract:cli.shared-proxy). Any other value is
	// reported by NginxModeProblem. See nginx_mode.go.
	Mode string `env:"NGINX_MODE"`
}

// RedisConfig holds Redis cache/queue configuration.
type RedisConfig struct {
	Enabled  bool   `env:"REDIS_ENABLED"`
	Version  string `env:"REDIS_VERSION"`   // 7-alpine
	Port     int    `env:"REDIS_PORT"`      // 6379
	Password string `env:"REDIS_PASSWORD"`  // empty = no auth
	Memory   string `env:"REDIS_MEMORY"`    // 512M
	CPU      string `env:"REDIS_CPU"`       // 0.5
	PoolSize int    `env:"REDIS_POOL_SIZE"` // 50 (prod default); use 20 for dev
	// Engine is REDIS_ENGINE: "valkey" or "redis" (lower case), or "" when not
	// set. Empty is resolved at generation (internal/compose cacheEngine): the
	// engine of the project lock's redis pin, else the mode default.
	Engine string `env:"REDIS_ENGINE"`
}

// MinioConfig holds MinIO S3-compatible object storage configuration.
type MinioConfig struct {
	Enabled        bool   `env:"MINIO_ENABLED"`
	Version        string `env:"MINIO_VERSION"`         // latest
	Port           int    `env:"MINIO_PORT"`            // 9000
	ConsolePort    int    `env:"MINIO_CONSOLE_PORT"`    // 9001
	RootUser       string `env:"MINIO_ROOT_USER"`       // minioadmin
	RootPassword   string `env:"MINIO_ROOT_PASSWORD"`   // minioadmin
	DefaultBuckets string `env:"MINIO_DEFAULT_BUCKETS"` // uploads,public,private,temp
	Region         string `env:"MINIO_REGION"`          // us-east-1
	S3AccessKey    string `env:"S3_ACCESS_KEY"`
	S3SecretKey    string `env:"S3_SECRET_KEY"`
	S3Bucket       string `env:"S3_BUCKET"`             // nself
	StorageVersion string `env:"STORAGE_VERSION"`       // 0.6.1
	StorageRoute   string `env:"STORAGE_ROUTE"`         // storage.{BD}
	ConsoleRoute   string `env:"STORAGE_CONSOLE_ROUTE"` // storage-console.{BD}
	MemLimit       string `env:"MINIO_MEMORY"`          // 1G
	CPULimit       string `env:"MINIO_CPU"`             // 0.5
}
