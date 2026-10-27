package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
)

// baseDomainRe matches valid domain names: alphanumeric start/end, interior
// may contain hyphens and dots. Minimum two characters.
var baseDomainRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9\-\.]*[a-zA-Z0-9]$`)

// insecurePatterns are substrings that, when found case-insensitively in a
// password, indicate an insecure value. Blocked in staging and production.
var insecurePatterns = []string{
	"password", "changeme", "secret", "admin",
	"12345", "qwerty", "default", "test",
	"postgres", "minioadmin", "hasura",
}

// reservedPorts maps port numbers to the service that owns them.
// Custom services must not bind to any of these.
var reservedPorts = map[int]string{
	80:   "HTTP",
	443:  "HTTPS",
	5432: "PostgreSQL",
	8080: "Hasura",
	4000: "Auth",
	6379: "Redis",
	9000: "MinIO",
	9001: "MinIO Console",
	7700: "MeiliSearch",
	3021: "nSelf Admin",
	1025: "Mailpit SMTP",
	8025: "Mailpit UI",
	3008: "Functions",
	5000: "MLflow",
}

// init registers all validators defined in this file with the central
// registry. File-alphabetical init() ordering within the package means
// registry.go's init() runs first (it has none), then validator.go's
// init() runs — no cross-file ordering dependency exists here.
func init() {
	register("env", validateEnv)
	register("passwords", func(cfg *Config) error {
		isProd := cfg.Env == "staging" || cfg.Env == "prod"
		return validatePasswords(cfg, isProd)
	})
	register("cors", func(cfg *Config) error {
		if cfg.Env == "prod" {
			return validateCORS(cfg)
		}
		return nil
	})
	register("jwt", func(cfg *Config) error {
		if cfg.Env == "staging" || cfg.Env == "prod" {
			return validateJWT(cfg)
		}
		return nil
	})
	register("port-conflicts", validatePortConflicts)
	register("duplicate-routes", validateDuplicateRoutes)
	register("base-domain", func(cfg *Config) error {
		return validateBaseDomain(cfg.BaseDomain, cfg.Env)
	})
	register("ports", func(cfg *Config) error {
		portFields := []struct {
			name  string
			value int
		}{
			{"POSTGRES_PORT", cfg.Postgres.Port},
			{"HASURA_PORT", cfg.Hasura.Port},
			{"AUTH_PORT", cfg.Auth.Port},
			{"NGINX_HTTP_PORT", cfg.Nginx.HTTPPort},
			{"NGINX_HTTPS_PORT", cfg.Nginx.SSLPort},
			{"MINIO_PORT", cfg.Minio.Port},
			{"MINIO_CONSOLE_PORT", cfg.Minio.ConsolePort},
			{"REDIS_PORT", cfg.Redis.Port},
			{"SEARCH_PORT", cfg.Search.Port},
			{"MAILPIT_SMTP_PORT", cfg.Mailpit.SMTPPort},
			{"MAILPIT_UI_PORT", cfg.Mailpit.UIPort},
		}
		for _, pf := range portFields {
			if pf.value == 0 {
				continue
			}
			if err := validatePort(pf.name, strconv.Itoa(pf.value)); err != nil {
				return err
			}
		}
		return nil
	})
}

// Validate checks cfg for security issues, port conflicts, and route
// collisions by running all registered validators. All failures are
// collected and returned together so the caller sees the full picture.
//
// Password and JWT validations are only enforced when Env is "staging" or
// "prod". In dev mode, weak passwords are acceptable (the caller is expected
// to auto-generate strong ones before reaching validation).
func Validate(cfg *Config) error {
	return runAll(cfg)
}

// validateEnv warns when Env is not a recognized value. After normalizeEnv
// has already run (in ApplyDefaults), an unrecognized value is anything that
// is not dev, staging, or prod. This is a soft error — the caller logs the
// warning and treats it as dev.
func validateEnv(cfg *Config) error {
	switch cfg.Env {
	case "dev", "staging", "prod":
		return nil
	default:
		// Per spec: warn, treat as dev. We return nil because the caller
		// (loader/defaults) already normalized to the raw value and the
		// warning is logged elsewhere. Validation does not block on this.
		return nil
	}
}

// validatePasswords checks all password fields for minimum length and
// (when checkPatterns is true) insecure patterns. checkPatterns is true
// for staging/prod and false for dev, where defaults like "minioadmin"
// or values containing "password" are acceptable.
func validatePasswords(cfg *Config, checkPatterns bool) error {
	// POSTGRES_PASSWORD — always required, min 16
	if err := checkPassword(cfg.Postgres.Password, "POSTGRES_PASSWORD", 16, checkPatterns); err != nil {
		return err
	}

	// HASURA_GRAPHQL_ADMIN_SECRET — always required, min 32
	if err := checkPassword(cfg.Hasura.AdminSecret, "HASURA_GRAPHQL_ADMIN_SECRET", 32, checkPatterns); err != nil {
		return err
	}

	// Optional service passwords — only enforced in staging/prod.
	// In dev, empty passwords are valid (Redis no-auth, Minio defaults, etc.)
	if checkPatterns {
		// REDIS_PASSWORD — only if Redis enabled, min 16
		if cfg.Redis.Enabled && cfg.Redis.Password != "" {
			if err := checkPassword(cfg.Redis.Password, "REDIS_PASSWORD", 16, checkPatterns); err != nil {
				return err
			}
		}

		// MINIO_ROOT_PASSWORD — only if Minio enabled, min 16
		if cfg.Minio.Enabled {
			if err := checkPassword(cfg.Minio.RootPassword, "MINIO_ROOT_PASSWORD", 16, checkPatterns); err != nil {
				return err
			}
		}

		// MEILISEARCH_MASTER_KEY — only if Search enabled with meilisearch engine, min 16
		if cfg.Search.Enabled && strings.EqualFold(cfg.Search.Engine, "meilisearch") {
			if err := checkPassword(cfg.Search.MeiliSearch.MasterKey, "MEILISEARCH_MASTER_KEY", 16, checkPatterns); err != nil {
				return err
			}
		}

		// TYPESENSE_API_KEY — only if Search enabled with typesense engine, min 16
		if cfg.Search.Enabled && strings.EqualFold(cfg.Search.Engine, "typesense") {
			if err := checkPassword(cfg.Search.Typesense.APIKey, "TYPESENSE_API_KEY", 16, checkPatterns); err != nil {
				return err
			}
		}

		// GRAFANA_ADMIN_PASSWORD — only if Monitoring enabled, min 16
		if cfg.Monitoring.Enabled {
			if err := checkPassword(cfg.Monitoring.GrafanaAdminPassword, "GRAFANA_ADMIN_PASSWORD", 16, checkPatterns); err != nil {
				return err
			}
		}

		// S3_SECRET_KEY — only if object storage enabled in v1.5+, min 16
		if compat.V15() && getEnvBool("OBJECT_STORAGE_ENABLED", false) {
			if err := checkPassword(cfg.Minio.S3SecretKey, "S3_SECRET_KEY", 16, checkPatterns); err != nil {
				return err
			}
		}
	}

	return nil
}

// checkPassword validates a single password field for minimum length and
// (when checkPatterns is true) insecure substrings. Returns a wrapped
// sentinel error on failure.
func checkPassword(value, fieldName string, minLen int, checkPatterns bool) error {
	if len(value) < minLen {
		return fmt.Errorf("%s must be at least %d characters: %w", fieldName, minLen, errs.ErrWeakPassword)
	}
	if checkPatterns && isInsecurePassword(value) {
		return fmt.Errorf("%s contains insecure pattern: %w", fieldName, errs.ErrInsecurePassword)
	}
	return nil
}

// isInsecurePassword returns true if value contains any of the blocked
// substrings (case-insensitive).
func isInsecurePassword(value string) bool {
	lower := strings.ToLower(value)
	for _, pattern := range insecurePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// validateCORS blocks wildcard "*" in HASURA_GRAPHQL_CORS_DOMAIN for prod.
func validateCORS(cfg *Config) error {
	if strings.Contains(cfg.Hasura.CORSDomain, "*") {
		return fmt.Errorf("HASURA_GRAPHQL_CORS_DOMAIN contains wildcard in production: %w", errs.ErrInvalidCORS)
	}
	return nil
}

// validateJWT checks that HASURA_JWT_KEY is at least 32 chars in staging/prod.
func validateJWT(cfg *Config) error {
	if len(cfg.Hasura.JWTKey) < 32 {
		return fmt.Errorf("HASURA_JWT_KEY must be at least 32 characters in %s environment: %w",
			cfg.Env, errs.ErrWeakPassword)
	}
	return nil
}

// validatePortConflicts checks all custom service ports against the reserved
// port list.
func validatePortConflicts(cfg *Config) error {
	for _, cs := range cfg.CustomServices {
		if cs.Port == 0 {
			continue
		}
		if svc, reserved := reservedPorts[cs.Port]; reserved {
			return fmt.Errorf("port %d reserved for %s — choose a different port for CS_%d: %w",
				cs.Port, svc, cs.Index, errs.ErrPortConflict)
		}
	}
	return nil
}

// validateDuplicateRoutes ensures no two custom services or frontend apps
// share the same route.
func validateDuplicateRoutes(cfg *Config) error {
	seen := make(map[string]string) // route -> owner description

	for _, cs := range cfg.CustomServices {
		if cs.Route == "" {
			continue
		}
		route := strings.ToLower(cs.Route)
		if owner, dup := seen[route]; dup {
			return fmt.Errorf("duplicate route %q in CS_%d and %s: %w",
				cs.Route, cs.Index, owner, errs.ErrDuplicateRoute)
		}
		seen[route] = fmt.Sprintf("CS_%d", cs.Index)
	}

	for _, fa := range cfg.FrontendApps {
		if fa.Route == "" {
			continue
		}
		route := strings.ToLower(fa.Route)
		if owner, dup := seen[route]; dup {
			return fmt.Errorf("duplicate route %q in FRONTEND_APP_%d and %s: %w",
				fa.Route, fa.Index, owner, errs.ErrDuplicateRoute)
		}
		seen[route] = fmt.Sprintf("FRONTEND_APP_%d", fa.Index)
	}

	return nil
}
