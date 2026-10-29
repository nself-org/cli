package config

// Purpose: additional Config validators for Redis, Hasura dev-mode, nginx rate/size inputs, MinIO credentials, base domains, and port range checks.
// Inputs: a *Config populated by the loader.
// Outputs: an error describing the first validation failure, or nil.
// Constraints: split out of validator.go as a pure move (CLI-R12); no behavior change.

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
)

// ValidateRedisPassword returns an error if Redis is enabled in staging/prod without a password.
func ValidateRedisPassword(cfg *Config) error {
	if !cfg.Redis.Enabled {
		return nil
	}
	if cfg.Env == "dev" || cfg.Env == "development" {
		return nil
	}
	if cfg.Redis.Password == "" {
		return fmt.Errorf("REDIS_PASSWORD must be set in staging/production environments.\nSet it in your .env file: REDIS_PASSWORD=<strong-password>")
	}
	return nil
}

// ValidateHasuraDevMode returns an error if HASURA_GRAPHQL_DEV_MODE=true in prod.
// When the block fires it emits a structured slog.Error so that operators with
// centralised log aggregation (Loki/Datadog) can alert on accidental dev-mode
// exposure. The error is always returned; the log is a side effect.
func ValidateHasuraDevMode(cfg *Config) error {
	if cfg.Env != "prod" && cfg.Env != "production" {
		return nil
	}
	if !cfg.Hasura.DevMode {
		return nil
	}
	// Emit structured audit log before returning the blocking error.
	// Fields: env, hostname (best-effort), and timestamp are included
	// automatically by slog's default handler.
	hostname, _ := os.Hostname()
	slog.Error("SEC-DEVMODE-01: HASURA_GRAPHQL_DEV_MODE=true blocked in production",
		"env", cfg.Env,
		"hostname", hostname,
		"remediation", "set HASURA_GRAPHQL_DEV_MODE=false in .env.prod",
	)
	return fmt.Errorf("HASURA_GRAPHQL_DEV_MODE must not be enabled in production\nset HASURA_GRAPHQL_DEV_MODE=false in your .env.prod file")
}

// nginx rate limit format: "30r/m", "10r/s", etc.
var rateRe = regexp.MustCompile(`^\d+(r/s|r/m)$`)

// nginx size format: "100M", "1G", "512K", "100"
var sizeRe = regexp.MustCompile(`^\d+[KMGkmg]?$`)

// ValidateNginxInputs validates raw nginx template inputs that are interpolated
// without escaping. Prevents nginx directive injection via env vars.
func ValidateNginxInputs(cfg *Config) error {
	if cfg.Nginx.AuthRateLimit != "" && !rateRe.MatchString(cfg.Nginx.AuthRateLimit) {
		return fmt.Errorf("AUTH_RATE_LIMIT %q has invalid format (expected e.g. '30r/m' or '10r/s')", cfg.Nginx.AuthRateLimit)
	}
	if cfg.Nginx.MaxBody != "" && !sizeRe.MatchString(cfg.Nginx.MaxBody) {
		return fmt.Errorf("NGINX_CLIENT_MAX_BODY_SIZE %q has invalid format (expected e.g. '100M' or '1G')", cfg.Nginx.MaxBody)
	}
	return nil
}

func init() {
	register("redis-password", ValidateRedisPassword)
	register("hasura-dev-mode", ValidateHasuraDevMode)
	register("nginx-inputs", ValidateNginxInputs)
	register("minio-credentials", ValidateMinioCredentials)
}

// ValidateMinioCredentials enforces strong MinIO credentials in staging and
// production. In dev, minioadmin defaults are accepted without error.
//
// Rules (staging/prod only):
//   - MINIO_ROOT_USER must not be empty or "minioadmin".
//   - MINIO_ROOT_PASSWORD must not be "minioadmin" and must be >=16 chars.
//
// In v1.5 MinIO is a plugin, so the rules apply only while the minio plugin is
// installed; without it the build refuses with E138, which names the real
// remedy, and a credential error first would cost the operator a second round
// (D-0685).
//
// Returns nil when MinIO is disabled or the environment is dev.
func ValidateMinioCredentials(cfg *Config) error {
	if !cfg.Minio.Enabled {
		return nil
	}
	// compat.V15(D-0685): credential guard before E138 for a missing minio plugin -> guard only for plugin-backed MinIO
	if compat.V15() && !minioPluginInstalled() {
		return nil
	}
	if cfg.Env != "prod" && cfg.Env != "staging" {
		return nil
	}
	if cfg.Minio.RootUser == "minioadmin" || cfg.Minio.RootUser == "" {
		return fmt.Errorf(
			"SECURITY: MINIO_ROOT_USER must not be 'minioadmin' in %s — set a strong unique value in .env",
			cfg.Env,
		)
	}
	if cfg.Minio.RootPassword == "minioadmin" || len(cfg.Minio.RootPassword) < 16 {
		return fmt.Errorf(
			"SECURITY: MINIO_ROOT_PASSWORD must be >=16 chars and not 'minioadmin' in %s",
			cfg.Env,
		)
	}
	return nil
}

// ── T08 ─────────────────────────────────────────────────────────────────────

// ValidateBaseDomain validates the BASE_DOMAIN value.
//
// Rules:
//   - Must match ^[a-zA-Z0-9][a-zA-Z0-9\-\.]*[a-zA-Z0-9]$ (rejects shell
//     metacharacters and single-character values).
//   - In env == "prod": "localhost" and any "*.local" suffix are rejected.
//   - In env == "dev" (the default): localhost and *.local are accepted — they
//     are normal for local development (TRAP-06).
func validateBaseDomain(domain string, env string) error {
	if !baseDomainRe.MatchString(domain) {
		return fmt.Errorf("BASE_DOMAIN %q contains invalid characters or format: %w",
			domain, errs.ErrInvalidDomain)
	}
	if env == "prod" {
		lower := strings.ToLower(domain)
		if lower == "localhost" {
			return fmt.Errorf("BASE_DOMAIN %q is not allowed in production: %w",
				domain, errs.ErrInvalidDomain)
		}
		if strings.HasSuffix(lower, ".local") {
			return fmt.Errorf("BASE_DOMAIN %q (.local suffix) is not allowed in production: %w",
				domain, errs.ErrInvalidDomain)
		}
	}
	return nil
}

// ── T09 ─────────────────────────────────────────────────────────────────────

// privilegedPortExceptions lists well-known ports below 1024 that are
// intentionally allowed without a warning (HTTP/HTTPS bound by Nginx).
var privilegedPortExceptions = map[int]bool{
	80:  true,
	443: true,
}

// ValidatePort validates a port environment variable value.
//
// Rules:
//   - Value must be a decimal integer (non-numeric → error).
//   - Range must be [1, 65535] (0 and negative → error; >65535 → error).
//   - Ports below 1024, except 80 and 443, return a warning-style error
//     so callers can surface the advisory without hard-blocking.
func validatePort(name, value string) error {
	n, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("%s value %q is not a valid integer: %w", name, value, errs.ErrInvalidPort)
	}
	if n < 1 || n > 65535 {
		return fmt.Errorf("%s value %d is out of the valid port range [1-65535]: %w",
			name, n, errs.ErrInvalidPort)
	}
	if n < 1024 && !privilegedPortExceptions[n] {
		// Privileged port — advisory, not a hard block. Return a descriptive
		// error so callers can decide whether to warn or reject.
		return fmt.Errorf("%s value %d is a privileged port (<1024); binding may require root: %w",
			name, n, errs.ErrInvalidPort)
	}
	return nil
}

// minioPluginInstalled reports whether the minio plugin manifest is installed
// in the global plugin directory (the same rule as compose's E138 check).
func minioPluginInstalled() bool {
	info, err := os.Stat(filepath.Join(DefaultPluginDir(), "minio", "plugin.json"))
	return err == nil && !info.IsDir()
}
