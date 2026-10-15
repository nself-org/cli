package config

import (
	"log/slog"
)

// ApplyDefaults fills every empty/zero field in cfg with the canonical default
// value. It never overrides a non-empty string, non-zero int, or explicitly-set
// boolean. Empty string "" is considered unset for string fields; zero is
// considered unset for int fields.
//
// Environment-specific overrides (Console, DevMode, CORS, BindIP, SSL) are
// applied after all static defaults.
func ApplyDefaults(cfg *Config) (*Config, error) {
	return applyDefaultsWithOptions(cfg, LoadOptions{})
}

// applyDefaultsWithOptions fills defaults while respecting the loader's
// explicit credential scope. Ordinary ApplyDefaults callers keep every guard.
func applyDefaultsWithOptions(cfg *Config, opts LoadOptions) (*Config, error) {
	applyDefaultsCore(cfg)
	if err := applyDefaultsPostgres(cfg); err != nil {
		return nil, err
	}
	if err := applyDefaultsHasura(cfg); err != nil {
		return nil, err
	}
	if err := applyDefaultsAuth(cfg); err != nil {
		return nil, err
	}
	applyDefaultsNginx(cfg)
	applyDefaultsSSL(cfg)
	applyDefaultsRedis(cfg)
	if err := applyDefaultsMinioWithOptions(cfg, opts); err != nil {
		return nil, err
	}
	applyDefaultsMailpit(cfg)
	applyDefaultsFunctions(cfg)
	applyDefaultsMLflow(cfg)
	applyDefaultsAdmin(cfg)
	applyDefaultsSearch(cfg)
	applyDefaultsMonitoring(cfg)
	applyDefaultsDocker(cfg)
	applyDefaultsStartStop(cfg)
	if err := applyDefaultsPlugins(cfg); err != nil {
		return nil, err
	}
	applyDefaultsBackup(cfg)
	applyDefaultsEmail(cfg)
	return cfg, nil
}

// applyDefaultsCore sets project-level defaults (name, domain, env).
func applyDefaultsCore(cfg *Config) {
	if cfg.ProjectName == "" {
		cfg.ProjectName = "myproject"
		slog.Debug("default", "key", "PROJECT_NAME", "value", cfg.ProjectName)
	}
	if cfg.BaseDomain == "" {
		cfg.BaseDomain = "local.nself.org"
		slog.Debug("default", "key", "BASE_DOMAIN", "value", cfg.BaseDomain)
	}
	if cfg.Env == "" {
		cfg.Env = "dev"
		slog.Debug("default", "key", "NSELF_ENV", "value", cfg.Env)
	}
	// Normalize env aliases (development->dev, production->prod, stage->staging)
	cfg.Env = normalizeEnv(cfg.Env)
}
