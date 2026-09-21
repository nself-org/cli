package config

// loader_helpers.go — small parsing helpers used by the config loader.
//
// Purpose: Three focused helpers that parse dynamic or structured env var sets
//          that cannot be reduced to a single getEnvOr call. Each lives here
//          to keep loader.go and loader_parse_env.go focused on orchestration
//          and field mapping respectively.
// Inputs:  os.Environ (collectPassthrough) and os.Getenv (parseInternalRoutes,
//          parseExtensionList) — no filesystem access.
// Outputs: collectPassthrough → map[string]string; parseInternalRoutes →
//          []InternalRoute; parseExtensionList → []string.
// Constraints: No side effects beyond returning values. Must not call
//              ApplyDefaults or touch the filesystem.
// SPORT:   cli/internal/config — decomposed from loader.go (T-E2-06).

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// collectPassthrough scans the full environment for dynamic env vars matching
// known prefixes (AUTH_*, REMOTE_SCHEMA_*, HASURA_EXTRA_*, HASURA_GRAPHQL_*)
// and returns them as a key-value map. These variables cannot be predefined in
// structs because users add them dynamically for OAuth providers, remote
// schemas, etc.
//
// HASURA_GRAPHQL_* and AUTH_* are namespace-wide prefixes (not single vars):
// loader_known_vars_core.go recognizes a superset of Hasura/Auth engine
// tuning vars (HASURA_GRAPHQL_ENABLE_ALLOWLIST, HASURA_GRAPHQL_NODE_LIMIT,
// AUTH_ANONYMOUS_USERS_ENABLED, etc.) purely to suppress "unknown env var"
// warnings, but buildHasuraService/buildAuthService only ever wrote a curated
// subset into the generated compose env — every other var in Hasura's/Auth's
// own namespace was silently dropped at `nself build` time even though it was
// present in .env (found live: HASURA_GRAPHQL_ENABLE_ALLOWLIST never reached
// the container). Capturing the full namespace here and merging it
// additively in the compose builders (existing curated keys always win) is
// safe because these are Hasura's/hasura-auth's own env vars — forwarding
// them through is exactly what the operator asked for by setting them.
func collectPassthrough(environ []string) map[string]string {
	prefixes := []string{
		"AUTH_",
		"REMOTE_SCHEMA_",
		"HASURA_EXTRA_",
		"HASURA_GRAPHQL_",
	}
	result := make(map[string]string)
	for _, env := range environ {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		for _, prefix := range prefixes {
			if strings.HasPrefix(parts[0], prefix) {
				result[parts[0]] = parts[1]
			}
		}
	}
	return result
}

// parseInternalRoutes parses INTERNAL_ROUTE_1 through INTERNAL_ROUTE_20
// environment variables into InternalRoute structs. Each route is defined by:
//
//	INTERNAL_ROUTE_N_NAME       — required (skip if empty)
//	INTERNAL_ROUTE_N_SUBDOMAIN
//	INTERNAL_ROUTE_N_TARGET     — e.g., hasura:8080
//	INTERNAL_ROUTE_N_RATE_ZONE  — default: general
//	INTERNAL_ROUTE_N_WEBSOCKET  — bool
func parseInternalRoutes() []InternalRoute {
	var routes []InternalRoute
	for i := 1; i <= 20; i++ {
		prefix := fmt.Sprintf("INTERNAL_ROUTE_%d_", i)
		name := os.Getenv(prefix + "NAME")
		if name == "" {
			continue
		}

		route := InternalRoute{
			Index:     i,
			Name:      name,
			Subdomain: os.Getenv(prefix + "SUBDOMAIN"),
			Target:    os.Getenv(prefix + "TARGET"),
			RateZone:  getEnvOr(prefix+"RATE_ZONE", "general"),
			WebSocket: getEnvBool(prefix+"WEBSOCKET", false),
		}
		routes = append(routes, route)
	}
	return routes
}

// parseHasuraJWTSecretJSON extracts the "key" and "type" fields from a
// HASURA_GRAPHQL_JWT_SECRET JSON blob (the format Hasura itself expects:
// {"type":"HS256","key":"..."}). Used by parseEnvToConfig (gap #4) so that a
// previously-persisted secret (written to .env.secrets by
// persistGeneratedSecrets) is read back into cfg.Hasura.JWTKey/JWTType instead
// of being silently ignored and regenerated on the next build. Returns
// ok=false when raw is empty or not valid JSON — callers must not treat that
// as an error, just as "no value available from this source".
func parseHasuraJWTSecretJSON(raw string) (key, typ string, ok bool) {
	if raw == "" {
		return "", "", false
	}
	var obj struct {
		Type string `json:"type"`
		Key  string `json:"key"`
	}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return "", "", false
	}
	if obj.Key == "" {
		return "", "", false
	}
	return obj.Key, obj.Type, true
}

// parseExtensionList parses a comma-separated extension list string into a slice.
// Trims whitespace from each element and removes empty entries.
func parseExtensionList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// hasExtensionCI reports whether extensions contains name, case-insensitively
// and with surrounding whitespace ignored. Used by parseEnvCore to make
// PGVECTOR_ENABLED purely additive against POSTGRES_EXTENSIONS.
func hasExtensionCI(extensions []string, name string) bool {
	for _, e := range extensions {
		if strings.EqualFold(strings.TrimSpace(e), name) {
			return true
		}
	}
	return false
}
