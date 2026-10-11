package config

// defaults_auth_nginx.go — default value application for Auth, Nginx and SSL.
//
// Purpose: Fill in unset Auth (including SMTP), Nginx and SSL fields on a loaded Config with the CLI's standard defaults, split out of defaults.go for file size.
// Inputs: a *Config already populated by the loader.
// Outputs: the same *Config with Auth, Nginx and SSL fields defaulted in place.
// Constraints: pure move from defaults.go (CLI-R12 Batch F); no behaviour change. Keep in sync with ApplyDefaults in defaults.go, which calls these in order.

import (
	"fmt"
	"log/slog"
)

// applyDefaultsAuth sets auth service connection, token, and SMTP defaults.
func applyDefaultsAuth(cfg *Config) error {
	applyDefaultsAuthConn(cfg)
	applyDefaultsAuthSMTP(cfg)
	return nil
}

// applyDefaultsAuthConn sets auth service version, port, token expiry, and resource limits.
func applyDefaultsAuthConn(cfg *Config) {
	if cfg.Auth.Version == "" {
		cfg.Auth.Version = "0.36.0"
		slog.Debug("default", "key", "AUTH_VERSION", "value", cfg.Auth.Version)
	}
	if cfg.Auth.Port == 0 {
		cfg.Auth.Port = 4000
		slog.Debug("default", "key", "AUTH_PORT", "value", fmt.Sprintf("%d", cfg.Auth.Port))
	}
	if cfg.Auth.ClientURL == "" {
		cfg.Auth.ClientURL = "http://localhost:3000"
		slog.Debug("default", "key", "AUTH_CLIENT_URL", "value", cfg.Auth.ClientURL)
	}
	if cfg.Auth.AccessTokenExpiry == 0 {
		cfg.Auth.AccessTokenExpiry = 900
		slog.Debug("default", "key", "AUTH_ACCESS_TOKEN_EXPIRES_IN", "value", fmt.Sprintf("%d", cfg.Auth.AccessTokenExpiry))
	}
	if cfg.Auth.RefreshTokenExpiry == 0 {
		cfg.Auth.RefreshTokenExpiry = 2592000
		slog.Debug("default", "key", "AUTH_REFRESH_TOKEN_EXPIRES_IN", "value", fmt.Sprintf("%d", cfg.Auth.RefreshTokenExpiry))
	}
	if cfg.Auth.Route == "" {
		cfg.Auth.Route = "auth"
		slog.Debug("default", "key", "AUTH_ROUTE", "value", cfg.Auth.Route)
	}
	if cfg.Auth.MemLimit == "" {
		cfg.Auth.MemLimit = "256m"
		slog.Debug("default", "key", "AUTH_MEM_LIMIT", "value", cfg.Auth.MemLimit)
	}
	if cfg.Auth.CPULimit == "" {
		cfg.Auth.CPULimit = "0.25"
		slog.Debug("default", "key", "AUTH_CPU_LIMIT", "value", cfg.Auth.CPULimit)
	}
	if cfg.Auth.LogLevel == "" {
		cfg.Auth.LogLevel = "info"
		slog.Debug("default", "key", "AUTH_LOG_LEVEL", "value", cfg.Auth.LogLevel)
	}
}

// applyDefaultsAuthSMTP sets auth service outbound SMTP relay defaults.
func applyDefaultsAuthSMTP(cfg *Config) {
	if cfg.Auth.SMTPHost == "" {
		cfg.Auth.SMTPHost = "mailpit"
		slog.Debug("default", "key", "AUTH_SMTP_HOST", "value", cfg.Auth.SMTPHost)
	}
	if cfg.Auth.SMTPPort == 0 {
		cfg.Auth.SMTPPort = 1025
		slog.Debug("default", "key", "AUTH_SMTP_PORT", "value", fmt.Sprintf("%d", cfg.Auth.SMTPPort))
	}
	if cfg.Auth.SMTPSender == "" {
		cfg.Auth.SMTPSender = "noreply@" + cfg.BaseDomain
		slog.Debug("default", "key", "AUTH_SMTP_SENDER", "value", cfg.Auth.SMTPSender)
	}
}

// applyDefaultsNginx sets nginx port, rate limit, and bind defaults.
func applyDefaultsNginx(cfg *Config) {
	if cfg.Nginx.Version == "" {
		cfg.Nginx.Version = "alpine"
		slog.Debug("default", "key", "NGINX_VERSION", "value", cfg.Nginx.Version)
	}
	if cfg.Nginx.HTTPPort == 0 {
		cfg.Nginx.HTTPPort = 80
		slog.Debug("default", "key", "NGINX_HTTP_PORT", "value", fmt.Sprintf("%d", cfg.Nginx.HTTPPort))
	}
	if cfg.Nginx.SSLPort == 0 {
		cfg.Nginx.SSLPort = 443
		slog.Debug("default", "key", "NGINX_SSL_PORT", "value", fmt.Sprintf("%d", cfg.Nginx.SSLPort))
	}
	if cfg.Nginx.MaxBody == "" {
		cfg.Nginx.MaxBody = "100M"
		slog.Debug("default", "key", "NGINX_MAX_BODY", "value", cfg.Nginx.MaxBody)
	}
	if cfg.Nginx.AuthRateLimit == "" {
		cfg.Nginx.AuthRateLimit = "30r/m"
		slog.Debug("default", "key", "AUTH_RATE_LIMIT", "value", cfg.Nginx.AuthRateLimit)
	}
	// BindIP: only default when unset. Preserves any user-set value (TRAP-03).
	if cfg.Nginx.BindIP == "" {
		if cfg.Env == "dev" {
			cfg.Nginx.BindIP = "127.0.0.1"
		} else {
			cfg.Nginx.BindIP = "0.0.0.0"
		}
		slog.Debug("default", "key", "NGINX_BIND_IP", "value", cfg.Nginx.BindIP)
	}
}

// applyDefaultsSSL sets SSL mode, provider, and WAF defaults.
func applyDefaultsSSL(cfg *Config) {
	if cfg.SSLMode == "" {
		if cfg.Env == "dev" {
			cfg.SSLMode = "local"
		} else {
			cfg.SSLMode = "letsencrypt"
		}
		slog.Debug("default", "key", "SSL_MODE", "value", cfg.SSLMode)
	}
	if cfg.SSLProvider == "" {
		cfg.SSLProvider = "cloudflare"
		slog.Debug("default", "key", "SSL_PROVIDER", "value", cfg.SSLProvider)
	}
	if cfg.WAFMode == "" {
		cfg.WAFMode = "off"
		slog.Debug("default", "key", "WAF_MODE", "value", cfg.WAFMode)
	}
}
