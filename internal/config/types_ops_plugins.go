package config

// types_ops_plugins.go — config struct fields for email, backup, licensing and plugins.
//
// Purpose: define the Email, Backup, License, Secrets, Tenant, DR, PluginPro, PluginSystem, CustomService, FrontendApp, RemoteSchema, InternalRoute and ApiDocs sub-config types embedded in Config, split out of types.go for file size.
// Inputs: none (pure type declarations, populated by the loader and defaults).
// Outputs: the sub-config types used as fields on Config.
// Constraints: pure move from types.go (CLI-R12 Batch F); no behaviour change. Keep in sync with defaults_plugins_backup.go, which populates these fields.

import "time"

// EmailConfig holds email provider configuration.
type EmailConfig struct {
	Provider            string `env:"EMAIL_PROVIDER"` // mailpit/elasticemail/sendgrid/postmark/mailgun/ses/smtp
	From                string `env:"EMAIL_FROM"`
	ElasticEmailAPIKey  string `env:"ELASTIC_EMAIL_API_KEY"`
	ElasticEmailAccount string `env:"ELASTIC_EMAIL_ACCOUNT_EMAIL"`
	SendGridAPIKey      string `env:"SENDGRID_API_KEY"`
	PostmarkAPIKey      string `env:"POSTMARK_API_KEY"`
	MailgunAPIKey       string `env:"MAILGUN_API_KEY"`
	MailgunDomain       string `env:"MAILGUN_DOMAIN"`
	AWSAccessKeyID      string `env:"AWS_ACCESS_KEY_ID"`
	AWSSecretAccessKey  string `env:"AWS_SECRET_ACCESS_KEY"`
	AWSRegion           string `env:"AWS_REGION"`
	SMTPHost            string `env:"SMTP_HOST"`
	SMTPPort            int    `env:"SMTP_PORT"`
	SMTPUser            string `env:"SMTP_USER"`
	SMTPPass            string `env:"SMTP_PASS"`
	SMTPSecure          bool   `env:"SMTP_SECURE"`
}

// BackupConfig holds backup and recovery configuration.
// Dir is read by internal/database/backup.go and restore.go for ad-hoc pg_dump/pg_restore.
// Scheduled, cloud, and retention features are managed via BACKUP_* env vars.
type BackupConfig struct {
	Dir           string `env:"BACKUP_DIR"` // ./backups — read by database/backup.go and restore.go
	Enabled       bool   `env:"BACKUP_ENABLED"`
	Schedule      string `env:"BACKUP_SCHEDULE"`       // legacy alias for BACKUP_SCHEDULE_FULL
	RetentionDays int    `env:"BACKUP_RETENTION_DAYS"` // legacy — use Daily/Weekly/Monthly instead
	CloudProvider string `env:"BACKUP_CLOUD_PROVIDER"` // legacy — use Remote instead

	// Cloud/remote storage
	Remote              string `env:"BACKUP_REMOTE"`                // rclone remote path, e.g. s3://bucket/path
	Encryption          bool   `env:"BACKUP_ENCRYPTION"`            // enable age encryption
	AgeRecipients       string `env:"BACKUP_AGE_RECIPIENTS"`        // age public key for encryption
	ScheduleFull        string `env:"BACKUP_SCHEDULE_FULL"`         // cron expr for full backups (default: 0 3 * * *)
	WALInterval         int    `env:"BACKUP_WAL_INTERVAL_SECONDS"`  // WAL archive interval (default: 60)
	RetentionDaily      int    `env:"BACKUP_RETENTION_DAILY"`       // keep last N daily backups (default: 7)
	RetentionWeekly     int    `env:"BACKUP_RETENTION_WEEKLY"`      // keep last N weekly backups (default: 4)
	RetentionMonthly    int    `env:"BACKUP_RETENTION_MONTHLY"`     // keep last N monthly backups (default: 12)
	RestoreTestSchedule string `env:"BACKUP_RESTORE_TEST_SCHEDULE"` // cron for restore tests (default: 0 5 * * 0)
	AlertOnFailure      bool   `env:"BACKUP_ALERT_ON_FAILURE"`      // send alert on backup failure
	S3AccessKeyID       string `env:"BACKUP_S3_ACCESS_KEY_ID"`
	S3SecretAccessKey   string `env:"BACKUP_S3_SECRET_ACCESS_KEY"`
	S3Region            string `env:"BACKUP_S3_REGION"`
	S3Endpoint          string `env:"BACKUP_S3_ENDPOINT"`

	// CriticalTables overrides database.DefaultCriticalTables for the backup
	// drill's smoke check (comma-separated table names, e.g.
	// "users,licenses,audit_logs,plugins"). Empty (the default) keeps the
	// np_-prefixed convention. Deployed schemas vary in whether they use
	// nSelf's np_ multi-app-isolation prefix, so the drill's critical-table
	// presence check must be project-configurable rather than hardcoded to
	// one convention — see .claude/qa/bugs/drill-critical-tables-naming.md.
	CriticalTables string `env:"BACKUP_CRITICAL_TABLES"`
}

// LicenseConfig holds license validation and grace period configuration.
//
// Offline grace-period lengths are NOT configurable here: they are fixed
// constants in internal/license/grace.go (GraceSoftThreshold,
// GraceHardThreshold), by design — see checker.go's exposure rationale
// (grace.go's GraceHardThreshold comment). A prior GraceDays field
// (env:"LICENSE_GRACE_DAYS") was declared here and referenced in a grace.go
// comment claiming the ladder was configurable, but nothing in the codebase
// ever read it. Both were removed together, P6-E12-W4-S4-T2, 2026-09.
type LicenseConfig struct {
	PingURL           string    `env:"LICENSE_PING_URL"`            // https://ping.nself.org
	CachePath         string    `env:"LICENSE_CACHE_PATH"`          // ~/.cache/nself/license.json
	CheckInterval     string    `env:"LICENSE_CHECK_INTERVAL"`      // 6h
	OfflineMode       bool      `env:"LICENSE_OFFLINE_MODE"`        // false
	PublicKeyOverride string    `env:"LICENSE_PUBLIC_KEY_OVERRIDE"` // hex-encoded Ed25519 pubkey for testing
	SunsetAt          time.Time `env:"LICENSE_SUNSET_AT"`           // optional hard cutoff; zero = no sunset
}

// SecretsConfig holds secrets management configuration.
type SecretsConfig struct {
	AgeKeyPath   string `env:"SECRETS_AGE_KEY_PATH"` // ~/.config/nself/age-key.txt
	DeployAgeKey string `env:"DEPLOY_AGE_KEY"`       // raw age private key for CI/CD
}

// TenantConfig holds multi-tenancy and billing configuration.
type TenantConfig struct {
	DefaultPlan             string `env:"TENANT_DEFAULT_PLAN"`               // basic
	DestroyBackupRetainDays int    `env:"TENANT_DESTROY_BACKUP_RETAIN_DAYS"` // 90
	StripeSecretKey         string `env:"STRIPE_SECRET_KEY"`
	StripeWebhookSecret     string `env:"STRIPE_WEBHOOK_SECRET"`
	StripeAPIVersion        string `env:"STRIPE_API_VERSION"` // 2024-04-10
}

// DRConfig holds disaster recovery configuration.
type DRConfig struct {
	SecondaryRegion string `env:"DR_SECONDARY_REGION"` // Hetzner region for standby
	StandbyHost     string `env:"DR_STANDBY_HOST"`     // IP/hostname of warm standby
	DrillSchedule   string `env:"DR_DRILL_SCHEDULE"`   // cron for DR drills (default: off)
}

// PluginProConfig holds per-plugin configuration for Pro plugins.
type PluginProConfig struct {
	NotifySecret    string `env:"NOTIFY_INTERNAL_SECRET"`
	NotifyPort      int    `env:"NOTIFY_PORT"` // 3712
	NotifyVAPIDPub  string `env:"NOTIFY_VAPID_PUBLIC_KEY"`
	NotifyVAPIDPriv string `env:"NOTIFY_VAPID_PRIVATE_KEY"`
	NotifyRoute     string `env:"NOTIFY_ROUTE"`
	CronSecret      string `env:"CRON_INTERNAL_SECRET"`
	CronPort        int    `env:"CRON_PORT"`                   // 3713
	CronRetention   int    `env:"CRON_RETENTION_DAYS"`         // 90
	AIMemLimit      string `env:"PLUGIN_AI_MEMORY_LIMIT"`      // 1g
	AICPULimit      string `env:"PLUGIN_AI_CPU_LIMIT"`         // 1.0
	MuxMemLimit     string `env:"PLUGIN_MUX_MEMORY_LIMIT"`     // 512m
	MuxCPULimit     string `env:"PLUGIN_MUX_CPU_LIMIT"`        // 0.5
	ClawMemLimit    string `env:"PLUGIN_CLAW_MEMORY_LIMIT"`    // 512m
	ClawCPULimit    string `env:"PLUGIN_CLAW_CPU_LIMIT"`       // 0.5
	DefaultMemLimit string `env:"PLUGIN_DEFAULT_MEMORY_LIMIT"` // 512m
	DefaultCPULimit string `env:"PLUGIN_DEFAULT_CPU_LIMIT"`    // 0.5
}

// PluginSystemConfig holds plugin system management configuration.
type PluginSystemConfig struct {
	Dir            string `env:"NSELF_PLUGIN_DIR"`         // ~/.nself/plugins
	Cache          string `env:"NSELF_PLUGIN_CACHE"`       // ~/.nself/cache/plugins
	Registry       string `env:"NSELF_PLUGIN_REGISTRY"`    // https://plugins.nself.org
	CacheTTL       int    `env:"NSELF_REGISTRY_CACHE_TTL"` // 300
	LicenseKey     string `env:"NSELF_PLUGIN_LICENSE_KEY"`
	SkipVerify     bool   `env:"NSELF_LICENSE_SKIP_VERIFY"`
	PingURL        string `env:"NSELF_PING_API_URL"` // https://ping.nself.org
	PricingURL     string `env:"NSELF_PRICING_URL"`  // https://nself.org/pricing
	InternalSecret string `env:"PLUGIN_INTERNAL_SECRET"`
}

// CustomService represents a user-defined custom service (CS_1..CS_10).
type CustomService struct {
	Index       int    // 1-10
	Name        string // parsed from CS_N
	Template    string // express-ts, fastapi, etc.
	Port        int
	Route       string // empty = internal only
	Public      bool
	Memory      string
	CPU         string
	TablePrefix string // CS_N_TABLE_PREFIX
	ExtraEnv    string // CS_N_ENV (raw key=val pairs, comma-separated)
	BuildPath   string // CS_N_PATH: overrides default ./services/{name} build context

	// HealthCheck is CS_N_HEALTHCHECK: a path (e.g. "/auth/health"), a full
	// "CMD ..." / "CMD-SHELL ..." override, or "disabled"/"none"/"false" to
	// omit the healthcheck entirely. Empty keeps the default GET /health.
	HealthCheck string

	// EnvPassthrough is CS_N_ENV_PASSTHROUGH: a comma-separated allowlist of
	// project .env var names to forward into this container in addition to
	// the fixed core set from coreEnvVars. CS_N_ENV still wins on conflict.
	EnvPassthrough string

	// Image is CS_N_IMAGE: a pre-built image reference (optionally digest-pinned,
	// e.g. "minio/minio:RELEASE.2024-01-16T16-07-38Z@sha256:...") to run instead
	// of building from a Dockerfile. When set, the compose generator emits
	// `image:` and omits `build:` entirely — mutually exclusive with
	// CS_N_PATH (G-013: closes the gap where a pinned third-party image had
	// no CS_N representation and had to be hand-authored into
	// docker-compose.override.yml).
	Image string

	// EnvFile is CS_N_ENV_FILE: a project-relative path to a dotenv-format
	// file whose KEY=VALUE lines are injected into this container. Unlike
	// CS_N_ENV (a single comma-joined line), a file has no comma/newline
	// escaping problem, so it is the right vehicle for many vars or values
	// that themselves contain commas (e.g. SMTP credentials). Precedence:
	// applied after CS_N_ENV_PASSTHROUGH, before CS_N_ENV (CS_N_ENV always
	// wins on conflict, per the existing coreEnvVars contract). Same
	// relative-path rules as BuildPath (no absolute paths, no "..").
	EnvFile string

	// Volumes is CS_N_VOLUMES: a comma-separated list of extra Docker volume
	// mounts in "host:container[:mode]" form (e.g.
	// "./email-templates:/app/templates:ro"), appended to the service's
	// generated volume list. Closes the gap where a required bind mount
	// (e.g. a template directory) had no CS_N representation.
	Volumes string
}

// FrontendApp represents a frontend application (FRONTEND_APP_1..FRONTEND_APP_20).
type FrontendApp struct {
	Index       int
	DisplayName string
	SystemName  string
	Port        int
	Route       string
	Framework   string
	TablePrefix string
	Image       string // FRONTEND_APP_N_IMAGE (optional docker image reference)
}

// RemoteSchema represents a Hasura Remote Schema configuration.
type RemoteSchema struct {
	Index   int
	Name    string
	URL     string
	Headers string // key:val,key:val
}

// InternalRoute represents an internal Nginx route (INTERNAL_ROUTE_1..INTERNAL_ROUTE_20).
type InternalRoute struct {
	Index     int
	Name      string // INTERNAL_ROUTE_N_NAME
	Subdomain string // INTERNAL_ROUTE_N_SUBDOMAIN
	Target    string // INTERNAL_ROUTE_N_TARGET (e.g., hasura:8080)
	RateZone  string // INTERNAL_ROUTE_N_RATE_ZONE (default: general)
	WebSocket bool   // INTERNAL_ROUTE_N_WEBSOCKET
}

// ApiDocsConfig holds the api_docs section from nself.yaml.
// Controls generation of the OpenAPI 3.1 spec and Scalar interactive docs page.
type ApiDocsConfig struct {
	Enabled         bool     `env:"API_DOCS_ENABLED"`      // default: true
	Path            string   `env:"API_DOCS_PATH"`         // serve path, default: /docs
	Title           string   `env:"API_DOCS_TITLE"`        // defaults to "<ProjectName> API"
	Theme           string   `env:"API_DOCS_THEME"`        // default | moon | purple | solarized
	AuthEnvVar      string   `env:"API_DOCS_AUTH_ENV_VAR"` // env var with bearer token for try-out
	HideEndpoints   []string // paths to exclude from the spec
	GraphQLEnabled  bool     `env:"API_DOCS_GRAPHQL_ENABLED"`  // default: true
	GraphQLEndpoint string   `env:"API_DOCS_GRAPHQL_ENDPOINT"` // default: /v1/graphql
}
