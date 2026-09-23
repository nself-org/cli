package config

// defaults_helpers.go — helper builders used alongside default application.
//
// Purpose: Construct derived values (JWT secret, database and service URLs) and generate secure random tokens, used by the loader and defaults appliers, split out of defaults.go for file size.
// Inputs: a *Config, or explicit connection parameters, depending on the function.
// Outputs: derived strings (URLs, secrets) or random byte-based tokens.
// Constraints: pure move from defaults.go (CLI-R12 Batch F); no behaviour change.

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// BuildJWTSecret constructs the HASURA_GRAPHQL_JWT_SECRET JSON string.
// If the environment variable HASURA_GRAPHQL_JWT_SECRET is already set,
// it is returned directly. Otherwise the secret is constructed from
// cfg.Hasura.JWTKey and cfg.Hasura.JWTType, which are the single source of
// truth also used by buildAuthService for AUTH_JWT_SECRET/AUTH_JWT_TYPE
// (JWT-ALGO-01) — this guarantees Hasura verifies tokens with the exact
// algorithm+key auth signed them with. A missing JWTKey is auto-generated
// (matching ApplyDefaults' HS256-default behavior); an empty JWTType defaults
// to HS256 for the same reason: no RSA keypair generator exists in this
// codebase, so RS256 can only be produced correctly when the caller already
// went through ApplyDefaults (which rejects RS256 without a supplied key).
func BuildJWTSecret(cfg *Config) (string, error) {
	// If the full JSON is already set in the environment, use it as-is.
	if existing := os.Getenv("HASURA_GRAPHQL_JWT_SECRET"); existing != "" {
		return existing, nil
	}

	jwtType := cfg.Hasura.JWTType
	if jwtType == "" {
		jwtType = "HS256" // JWT-ALGO-01: HS256 is the safe zero-config default
	}

	jwtKey := cfg.Hasura.JWTKey
	if jwtKey == "" {
		// Auto-generate if not provided in any environment.
		// Staging/prod should ideally set this explicitly, but auto-gen
		// is safer than failing — the user can always override in .env.
		secret, err := generateSecureRandom(44)
		if err != nil {
			return "", fmt.Errorf("generating JWT key: %w", err)
		}
		jwtKey = secret
		cfg.Hasura.JWTKey = jwtKey
	}

	obj := map[string]string{"type": jwtType, "key": jwtKey}
	b, _ := json.Marshal(obj)
	return string(b), nil
}

// DatabaseURL returns the computed PostgreSQL connection string using internal
// container networking (always port 5432, host "postgres"). The password is
// percent-encoded per RFC 3986 for safe URL inclusion (see URLUserInfo).
func (cfg *Config) DatabaseURL() string {
	return fmt.Sprintf("postgresql://%s@%s:5432/%s",
		URLUserInfo(cfg.Postgres.User, cfg.Postgres.Password),
		cfg.Postgres.Host,
		cfg.Postgres.DB,
	)
}

// EmbeddedPGDatabaseURL returns a PostgreSQL DSN that connects via the
// Unix-domain socket bridge created by the pglite/wasmtime embedded runtime.
// The host field is the runtimeDir path; sslmode=disable is required because
// the embedded runtime does not perform TLS termination.
//
// Use this DSN only when cfg.EmbeddedPG is true and runtimeDir is the
// directory passed to embedded.NewEmbeddedPGRuntime.
func (cfg *Config) EmbeddedPGDatabaseURL(runtimeDir string) string {
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}
	// The host= DSN key accepts a directory path for UDS connections in libpq.
	return fmt.Sprintf("host=%s dbname=%s sslmode=disable", runtimeDir, db)
}

// BuildServiceURL constructs a full HTTPS URL for a service subdomain against
// the given baseDomain. If baseDomain already starts with "subdomain.", that
// prefix is stripped first to avoid double-prefixing (e.g. "auth.auth.example.com").
//
// Examples:
//
//	BuildServiceURL("auth", "auth.example.com") → "https://auth.example.com"
//	BuildServiceURL("auth", "example.com")      → "https://auth.example.com"
func BuildServiceURL(subdomain, baseDomain string) string {
	cleanDomain := strings.TrimPrefix(baseDomain, subdomain+".")
	return "https://" + subdomain + "." + cleanDomain
}

// generateSecureRandom returns a crypto/rand base64url string of the given
// length. Used for auto-generating JWT keys and other secrets in dev mode.
func generateSecureRandom(length int) (string, error) {
	// Allocate enough bytes to guarantee the encoded output is >= length.
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating secret: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(b)
	if len(encoded) < length {
		return encoded, nil
	}
	return encoded[:length], nil
}
