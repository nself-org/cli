package config

// loader_parse_env_ops.go — env var -> Config field mapping: Monitoring, Email,
// Backup, Plugin Pro, Plugin System, Docker, Start/Stop. Split from
// loader_parse_env.go (T-P6-E2-W1-S1-T3).
// Inputs:  os.Environ, read into the cfg passed in by parseEnvToConfig.
// Outputs: none — mutates cfg in place.
// Constraints: pure os.Getenv reads only, same rules as loader_parse_env.go.

import "os"

// parseEnvOps fills the Monitoring/Email/Backup/PluginPro/PluginSystem/Docker/
// Start-Stop fields.
func parseEnvOps(cfg *Config) {
	// ── Monitoring ───────────────────────────────────────────────────
	cfg.Monitoring = MonitoringConfig{
		Enabled:              getEnvBool("MONITORING_ENABLED", false),
		PrometheusEnabled:    getEnvBool("PROMETHEUS_ENABLED", false),
		PrometheusPort:       getEnvInt("PROMETHEUS_PORT", 0),
		GrafanaEnabled:       getEnvBool("GRAFANA_ENABLED", false),
		GrafanaPort:          getEnvInt("GRAFANA_PORT", 0),
		GrafanaAdminUser:     os.Getenv("GRAFANA_ADMIN_USER"),
		GrafanaAdminPassword: os.Getenv("GRAFANA_ADMIN_PASSWORD"),
		GrafanaRoute:         os.Getenv("GRAFANA_ROUTE"),
		LokiEnabled:          getEnvBool("LOKI_ENABLED", false),
		LokiPort:             getEnvInt("LOKI_PORT", 0),
		PromtailEnabled:      getEnvBool("PROMTAIL_ENABLED", false),
		TempoEnabled:         getEnvBool("TEMPO_ENABLED", false),
		TempoPort:            getEnvInt("TEMPO_PORT", 0),
		AlertmanagerEnabled:  getEnvBool("ALERTMANAGER_ENABLED", false),
		AlertmanagerPort:     getEnvInt("ALERTMANAGER_PORT", 0),
		CadvisorEnabled:      getEnvBool("CADVISOR_ENABLED", false),
		CadvisorPort:         getEnvInt("CADVISOR_PORT", 0),
		NodeExporterEnabled:  getEnvBool("NODE_EXPORTER_ENABLED", false),
		NodeExporterPort:     getEnvInt("NODE_EXPORTER_PORT", 0),
		PGExporterEnabled:    getEnvBool("POSTGRES_EXPORTER_ENABLED", false),
		PGExporterPort:       getEnvInt("POSTGRES_EXPORTER_PORT", 0),
		RedisExporterEnabled: getEnvBool("REDIS_EXPORTER_ENABLED", false),
		RedisExporterPort:    getEnvInt("REDIS_EXPORTER_PORT", 0),
	}

	// ── Email ────────────────────────────────────────────────────────
	cfg.Email = EmailConfig{
		Provider:            os.Getenv("EMAIL_PROVIDER"),
		From:                os.Getenv("EMAIL_FROM"),
		ElasticEmailAPIKey:  os.Getenv("ELASTIC_EMAIL_API_KEY"),
		ElasticEmailAccount: os.Getenv("ELASTIC_EMAIL_ACCOUNT_EMAIL"),
		SendGridAPIKey:      os.Getenv("SENDGRID_API_KEY"),
		PostmarkAPIKey:      os.Getenv("POSTMARK_API_KEY"),
		MailgunAPIKey:       os.Getenv("MAILGUN_API_KEY"),
		MailgunDomain:       os.Getenv("MAILGUN_DOMAIN"),
		AWSAccessKeyID:      os.Getenv("AWS_ACCESS_KEY_ID"),
		AWSSecretAccessKey:  os.Getenv("AWS_SECRET_ACCESS_KEY"),
		AWSRegion:           os.Getenv("AWS_REGION"),
		SMTPHost:            os.Getenv("SMTP_HOST"),
		SMTPPort:            getEnvInt("SMTP_PORT", 0),
		SMTPUser:            os.Getenv("SMTP_USER"),
		SMTPPass:            os.Getenv("SMTP_PASS"),
		SMTPSecure:          getEnvBool("SMTP_SECURE", false),
	}

	// ── Backup ───────────────────────────────────────────────────────
	cfg.Backup = BackupConfig{
		Enabled:             getEnvBool("BACKUP_ENABLED", false),
		Dir:                 os.Getenv("BACKUP_DIR"),
		Schedule:            os.Getenv("BACKUP_SCHEDULE"),
		RetentionDays:       getEnvInt("BACKUP_RETENTION_DAYS", 0),
		CloudProvider:       os.Getenv("BACKUP_CLOUD_PROVIDER"),
		Remote:              os.Getenv("BACKUP_REMOTE"),
		Encryption:          getEnvBool("BACKUP_ENCRYPTION", false),
		AgeRecipients:       os.Getenv("BACKUP_AGE_RECIPIENTS"),
		ScheduleFull:        os.Getenv("BACKUP_SCHEDULE_FULL"),
		WALInterval:         getEnvInt("BACKUP_WAL_INTERVAL_SECONDS", 0),
		RetentionDaily:      getEnvInt("BACKUP_RETENTION_DAILY", 0),
		RetentionWeekly:     getEnvInt("BACKUP_RETENTION_WEEKLY", 0),
		RetentionMonthly:    getEnvInt("BACKUP_RETENTION_MONTHLY", 0),
		RestoreTestSchedule: os.Getenv("BACKUP_RESTORE_TEST_SCHEDULE"),
		AlertOnFailure:      getEnvBool("BACKUP_ALERT_ON_FAILURE", true),
		// S3AccessKeyID/S3SecretAccessKey: BACKUP_S3_ACCESS_KEY_ID /
		// BACKUP_S3_SECRET_ACCESS_KEY is the canonical pair, but real project
		// .env files (ntask/backend/.env.example, R2-backed) use the shorter
		// BACKUP_ACCESS_KEY / BACKUP_SECRET_KEY names alongside
		// BACKUP_S3_ENDPOINT — those were on loader_known_vars_ops.go's list
		// (so no "unknown env var" warning fired) but never actually read
		// into this struct, so a project configured that way had remote
		// backup upload silently do nothing. Both names are accepted here;
		// the canonical BACKUP_S3_* pair wins if both happen to be set.
		S3AccessKeyID:     firstNonEmpty(os.Getenv("BACKUP_S3_ACCESS_KEY_ID"), os.Getenv("BACKUP_ACCESS_KEY")),
		S3SecretAccessKey: firstNonEmpty(os.Getenv("BACKUP_S3_SECRET_ACCESS_KEY"), os.Getenv("BACKUP_SECRET_KEY")),
		S3Region:          os.Getenv("BACKUP_S3_REGION"),
		S3Endpoint:        os.Getenv("BACKUP_S3_ENDPOINT"),
		CriticalTables:    os.Getenv("BACKUP_CRITICAL_TABLES"),
	}

	cfg.DR = DRConfig{
		SecondaryRegion: os.Getenv("DR_SECONDARY_REGION"),
		StandbyHost:     os.Getenv("DR_STANDBY_HOST"),
		DrillSchedule:   os.Getenv("DR_DRILL_SCHEDULE"),
	}

	// Plugin port defaults (3712=notify, 3713=cron) are set in ApplyDefaults() when port==0.
	// ── Plugin Pro Configuration ─────────────────────────────────────
	cfg.PluginConfig = PluginProConfig{
		NotifySecret:    os.Getenv("NOTIFY_INTERNAL_SECRET"),
		NotifyPort:      getEnvInt("NOTIFY_PORT", 0),
		NotifyVAPIDPub:  os.Getenv("NOTIFY_VAPID_PUBLIC_KEY"),
		NotifyVAPIDPriv: os.Getenv("NOTIFY_VAPID_PRIVATE_KEY"),
		NotifyRoute:     os.Getenv("NOTIFY_ROUTE"),
		CronSecret:      os.Getenv("CRON_INTERNAL_SECRET"),
		CronPort:        getEnvInt("CRON_PORT", 0),
		CronRetention:   getEnvInt("CRON_RETENTION_DAYS", 0),
		AIMemLimit:      os.Getenv("PLUGIN_AI_MEMORY_LIMIT"),
		AICPULimit:      os.Getenv("PLUGIN_AI_CPU_LIMIT"),
		MuxMemLimit:     os.Getenv("PLUGIN_MUX_MEMORY_LIMIT"),
		MuxCPULimit:     os.Getenv("PLUGIN_MUX_CPU_LIMIT"),
		ClawMemLimit:    os.Getenv("PLUGIN_CLAW_MEMORY_LIMIT"),
		ClawCPULimit:    os.Getenv("PLUGIN_CLAW_CPU_LIMIT"),
		DefaultMemLimit: os.Getenv("PLUGIN_DEFAULT_MEMORY_LIMIT"),
		DefaultCPULimit: os.Getenv("PLUGIN_DEFAULT_CPU_LIMIT"),
	}

	// ── Plugin System ────────────────────────────────────────────────
	cfg.PluginSystem = PluginSystemConfig{
		Dir:            os.Getenv("NSELF_PLUGIN_DIR"),
		Cache:          os.Getenv("NSELF_PLUGIN_CACHE"),
		Registry:       os.Getenv("NSELF_PLUGIN_REGISTRY"),
		CacheTTL:       getEnvInt("NSELF_REGISTRY_CACHE_TTL", 0),
		LicenseKey:     os.Getenv("NSELF_PLUGIN_LICENSE_KEY"),
		SkipVerify:     getEnvBool("NSELF_LICENSE_SKIP_VERIFY", false),
		PingURL:        os.Getenv("NSELF_PING_API_URL"),
		PricingURL:     os.Getenv("NSELF_PRICING_URL"),
		InternalSecret: os.Getenv("PLUGIN_INTERNAL_SECRET"),
	}

	// ── Docker ───────────────────────────────────────────────────────
	cfg.DockerNetwork = os.Getenv("DOCKER_NETWORK")
	cfg.DockerLogMaxSize = os.Getenv("DOCKER_LOG_MAX_SIZE")
	cfg.DockerLogMaxFile = os.Getenv("DOCKER_LOG_MAX_FILE")
	cfg.DockerStopGrace = os.Getenv("DOCKER_STOP_GRACE_PERIOD")
	cfg.DockerBuildTimeout = getEnvInt("NSELF_DOCKER_BUILD_TIMEOUT", 0)

	// ── Start/Stop ───────────────────────────────────────────────────
	cfg.StartMode = os.Getenv("NSELF_START_MODE")
	cfg.HealthCheckTimeout = getEnvInt("NSELF_HEALTH_CHECK_TIMEOUT", 0)
	cfg.HealthCheckInterval = getEnvInt("NSELF_HEALTH_CHECK_INTERVAL", 0)
	cfg.HealthCheckRequired = getEnvInt("NSELF_HEALTH_CHECK_REQUIRED", 0)
	cfg.CleanupOnStart = os.Getenv("NSELF_CLEANUP_ON_START")
	cfg.AllowExposedPorts = getEnvBool("NSELF_ALLOW_EXPOSED_PORTS", false)
	cfg.ParallelLimit = getEnvInt("NSELF_PARALLEL_LIMIT", 0)
	cfg.LogLevel = os.Getenv("NSELF_LOG_LEVEL")
	cfg.SkipHealthChecks = getEnvBool("NSELF_SKIP_HEALTH_CHECKS", false)
	cfg.StopTimeout = getEnvInt("NSELF_STOP_TIMEOUT", 0)
}

// firstNonEmpty returns the first non-empty string among values, or "" if
// all are empty. Used for accepting an alias env var name alongside a
// canonical one without silently preferring whichever happens to be read
// last.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
