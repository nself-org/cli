package config

// Purpose: refuse remote builds and snapshots that depend on process-only config.
// Inputs: resolved cascade keys and the current process environment.
// Outputs: an actionable error naming keys only, or nil.
// Constraints: never forward process values; file presence wins, even if empty.

import (
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/compat"
)

// localOnlyConfigKeys are process variables that point the local CLI at its own
// directories or registry, or set how the CLI prints; the remote host never reads them.
var localOnlyConfigKeys = map[string]bool{
	"NO_COLOR":                 true, // the MCP and HTTP invoker sets it on every child (internal/invoke)
	"AI_AUTO_INSTALL":          true,
	"NSELF_PLUGIN_DIR":         true,
	"NSELF_PLUGIN_CACHE":       true,
	"NSELF_PLUGIN_REGISTRY":    true,
	"NSELF_REGISTRY_CACHE_TTL": true,
}

// remoteExtraConfigKeys are build-shaping keys read outside the known-vars
// schema; a process-only value for them must also be refused.
var remoteExtraConfigKeys = map[string]bool{
	"SSL_PROVIDER":        true,
	"SSL_WILDCARD_DOMAIN": true,
	"CLOUDFLARE_API_KEY":  true,
	"WAF_MODE":            true,
}

// ValidateRemoteEnv requires every populated config key in the process to
// exist in the remote file cascade. ENV is the explicit deploy target selector,
// set by deploy itself; it is not a credential or a value to copy to the host.
// Keys in localOnlyConfigKeys steer the local CLI only and never shape a remote build.
func ValidateRemoteEnv(fromFiles map[string]string) error {
	known := make(map[string]bool, len(knownEnvVars))
	for _, key := range knownEnvVars {
		known[key] = true
	}
	var missing []string
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || value == "" || key == "ENV" || localOnlyConfigKeys[key] {
			continue
		}
		if _, present := fromFiles[key]; present {
			continue
		}
		if known[key] || remoteExtraConfigKeys[key] || remoteDynamicConfigKey(key) {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	err := fmt.Errorf("remote deploy refused: process-only config keys %s; put these keys in an env file in the remote target's cascade (.env, .env.<target>, or .env.secrets); process values are never shipped", strings.Join(missing, ", "))
	// compat.V15(D-0260): process-only config keys shape the remote build with a warning -> the remote deploy is refused
	if !compat.V15() {
		slog.Warn("remote deploy: process-only config keys shape the build but are not shipped to the host; v1.5 refuses this", "keys", strings.Join(missing, ", "))
		return nil
	}
	return err
}

// RemoteConfigKey reports whether key is a build-shaping config key outside the
// known-vars list (extra keys and dynamic collections); tests use it to isolate.
func RemoteConfigKey(key string) bool {
	return remoteExtraConfigKeys[key] || remoteDynamicConfigKey(key)
}

// remoteDynamicConfigKey recognizes the dynamic collections and passthrough
// namespaces consumed by the loader, rather than arbitrary shell credentials.
func remoteDynamicConfigKey(key string) bool {
	for _, prefix := range []string{"CS_", "FRONTEND_APP_", "INTERNAL_ROUTE_", "AUTH_", "REMOTE_SCHEMA_", "HASURA_EXTRA_", "HASURA_GRAPHQL_"} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}
