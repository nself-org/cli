package config

// defaults_postgres_hasura.go — default value application for Postgres and Hasura.
//
// Purpose: Fill in unset Postgres and Hasura fields on a loaded Config with the CLI's standard defaults, split out of defaults.go for file size.
// Inputs: a *Config already populated by the loader.
// Outputs: the same *Config with Postgres and Hasura fields defaulted in place.
// Constraints: pure move from defaults.go (CLI-R12 Batch F); no behaviour change. Keep in sync with ApplyDefaults in defaults.go, which calls these in order.

import (
	"fmt"
	"log/slog"
	"strings"
)

// applyDefaultsPostgres sets PostgreSQL connection and resource defaults.
func applyDefaultsPostgres(cfg *Config) error {
	if cfg.Postgres.Version == "" {
		cfg.Postgres.Version = "16-alpine"
		slog.Debug("default", "key", "POSTGRES_VERSION", "value", cfg.Postgres.Version)
	}
	if cfg.Postgres.Host == "" {
		cfg.Postgres.Host = "postgres"
		slog.Debug("default", "key", "POSTGRES_HOST", "value", cfg.Postgres.Host)
	}
	if cfg.Postgres.Port == 0 {
		cfg.Postgres.Port = 5432
		slog.Debug("default", "key", "POSTGRES_PORT", "value", fmt.Sprintf("%d", cfg.Postgres.Port))
	}
	if cfg.Postgres.DB == "" {
		cfg.Postgres.DB = "nself"
		slog.Debug("default", "key", "POSTGRES_DB", "value", cfg.Postgres.DB)
	}
	if cfg.Postgres.User == "" {
		cfg.Postgres.User = "postgres"
		slog.Debug("default", "key", "POSTGRES_USER", "value", cfg.Postgres.User)
	}
	if cfg.Postgres.Password == "" {
		secret, err := generateSecureRandom(24)
		if err != nil {
			return fmt.Errorf("generating POSTGRES_PASSWORD: %w", err)
		}
		cfg.Postgres.Password = secret
		slog.Debug("default", "key", "POSTGRES_PASSWORD", "value", "[generated]")
	}
	if len(cfg.Postgres.Extensions) == 0 {
		cfg.Postgres.Extensions = []string{"uuid-ossp"}
		slog.Debug("default", "key", "POSTGRES_EXTENSIONS", "value", cfg.Postgres.Extensions)
	}
	if cfg.Postgres.ExposePort == "" {
		cfg.Postgres.ExposePort = "auto"
		slog.Debug("default", "key", "POSTGRES_EXPOSE_PORT", "value", cfg.Postgres.ExposePort)
	}
	if cfg.Postgres.MemLimit == "" {
		cfg.Postgres.MemLimit = "2g"
		slog.Debug("default", "key", "POSTGRES_MEM_LIMIT", "value", cfg.Postgres.MemLimit)
	}
	if cfg.Postgres.CPULimit == "" {
		cfg.Postgres.CPULimit = "2.0"
		slog.Debug("default", "key", "POSTGRES_CPU_LIMIT", "value", cfg.Postgres.CPULimit)
	}
	return nil
}

// applyDefaultsHasura sets Hasura engine defaults by delegating to credential and config helpers.
func applyDefaultsHasura(cfg *Config) error {
	if err := applyDefaultsHasuraCreds(cfg); err != nil {
		return err
	}
	applyDefaultsHasuraConfig(cfg)
	return nil
}

// applyDefaultsHasuraCreds generates Hasura admin secret and JWT key when missing.
func applyDefaultsHasuraCreds(cfg *Config) error {
	if cfg.Hasura.Version == "" {
		cfg.Hasura.Version = "v2.44.0"
		slog.Debug("default", "key", "HASURA_VERSION", "value", cfg.Hasura.Version)
	}
	if cfg.Hasura.AdminSecret == "" {
		secret, err := generateSecureRandom(44)
		if err != nil {
			return fmt.Errorf("generating HASURA_GRAPHQL_ADMIN_SECRET: %w", err)
		}
		cfg.Hasura.AdminSecret = secret
		slog.Debug("default", "key", "HASURA_GRAPHQL_ADMIN_SECRET", "value", "[generated]")
	}
	// JWT-ALGO-01: the algorithm auth SIGNS tokens with and the algorithm Hasura
	// VERIFIES them with must always match — see BuildJWTSecret and
	// buildAuthService/buildHasuraService, which both derive AUTH_JWT_TYPE and
	// HASURA_GRAPHQL_JWT_SECRET's "type" field from this single cfg.Hasura.JWTType
	// value. Only honor an explicit user-supplied HASURA_JWT_TYPE/AUTH_JWT_TYPE.
	// When unset, default to HS256: this codebase has no RSA keypair generator,
	// so auto-generating cfg.Hasura.JWTKey as a plain random string and labeling
	// it RS256 previously produced a key Hasura could never actually verify
	// with, breaking all auth on a fresh install. HS256 uses the same random
	// secret for both signing and verification, which is exactly what
	// generateSecureRandom produces, so it is the only safe zero-config default.
	if cfg.Hasura.JWTType == "" {
		cfg.Hasura.JWTType = "HS256"
		slog.Debug("default", "key", "HASURA_GRAPHQL_JWT_TYPE", "value", cfg.Hasura.JWTType)
	} else if strings.EqualFold(cfg.Hasura.JWTType, "RS256") && cfg.Hasura.JWTKey == "" {
		// The user asked for RS256 but supplied no key material (HASURA_JWT_KEY /
		// AUTH_JWT_KEY). We cannot synthesize a valid RSA keypair, so generating a
		// random string here would reproduce JWT-ALGO-01. Fail loudly instead of
		// silently shipping broken auth.
		return fmt.Errorf("HASURA_JWT_TYPE=RS256 requires HASURA_JWT_KEY to be set " +
			"(an RSA public key/JWKS) — nSelf does not generate RSA keypairs automatically; " +
			"set HASURA_JWT_TYPE=HS256 to use an auto-generated shared secret instead")
	}
	if cfg.Hasura.JWTKey == "" {
		secret, err := generateSecureRandom(44)
		if err != nil {
			return fmt.Errorf("generating HASURA_GRAPHQL_JWT_KEY: %w", err)
		}
		cfg.Hasura.JWTKey = secret
		slog.Debug("default", "key", "HASURA_GRAPHQL_JWT_KEY", "value", "[generated]")
	}
	return nil
}

// applyDefaultsHasuraConfig sets Hasura routing, resource limits, and env-specific toggles.
func applyDefaultsHasuraConfig(cfg *Config) {
	if cfg.Hasura.Route == "" {
		cfg.Hasura.Route = "api"
		slog.Debug("default", "key", "HASURA_ROUTE", "value", cfg.Hasura.Route)
	}
	if cfg.Hasura.Port == 0 {
		cfg.Hasura.Port = 8080
		slog.Debug("default", "key", "HASURA_PORT", "value", fmt.Sprintf("%d", cfg.Hasura.Port))
	}
	if cfg.Hasura.MemLimit == "" {
		cfg.Hasura.MemLimit = "1g"
		slog.Debug("default", "key", "HASURA_MEM_LIMIT", "value", cfg.Hasura.MemLimit)
	}
	if cfg.Hasura.CPULimit == "" {
		cfg.Hasura.CPULimit = "1.0"
		slog.Debug("default", "key", "HASURA_CPU_LIMIT", "value", cfg.Hasura.CPULimit)
	}
	if cfg.Hasura.LogLevel == "" {
		cfg.Hasura.LogLevel = "warn"
		slog.Debug("default", "key", "HASURA_GRAPHQL_LOG_LEVEL", "value", cfg.Hasura.LogLevel)
	}
	// Env-specific: dev/staging get console+devmode, prod forces them off.
	if cfg.Env == "prod" {
		cfg.Hasura.Console = false
		cfg.Hasura.DevMode = false
	} else {
		if !cfg.Hasura.Console {
			cfg.Hasura.Console = true
		}
		if !cfg.Hasura.DevMode {
			cfg.Hasura.DevMode = true
		}
	}
	// CORS: dev gets localhost wildcard, non-dev gets domain wildcard.
	if cfg.Hasura.CORSDomain == "" {
		if cfg.Env == "dev" {
			cfg.Hasura.CORSDomain = "http://localhost:*"
		} else {
			cfg.Hasura.CORSDomain = "https://*." + cfg.BaseDomain
		}
		slog.Debug("default", "key", "HASURA_GRAPHQL_CORS_DOMAIN", "value", cfg.Hasura.CORSDomain)
	}
}
