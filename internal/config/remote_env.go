package config

// Purpose: refuse remote builds and snapshots that depend on process-only config.
// Inputs: resolved cascade keys and the current process environment.
// Outputs: an actionable error naming keys only, or nil.
// Constraints: never forward process values; file presence wins, even if empty.

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// ValidateRemoteEnv requires every populated config key in the process to
// exist in the remote file cascade. ENV is the explicit deploy target selector,
// set by deploy itself; it is not a credential or a value to copy to the host.
// AI_AUTO_INSTALL only controls local CLI tool installation, not build config.
func ValidateRemoteEnv(fromFiles map[string]string) error {
	known := make(map[string]bool, len(knownEnvVars))
	for _, key := range knownEnvVars {
		known[key] = true
	}
	var missing []string
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || value == "" || key == "ENV" || key == "AI_AUTO_INSTALL" {
			continue
		}
		if _, present := fromFiles[key]; present {
			continue
		}
		if known[key] || remoteDynamicConfigKey(key) {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("remote deploy refused: process-only config keys %s; put these keys in an env file in the remote target's cascade (.env, .env.<target>, or .env.secrets); process values are never shipped", strings.Join(missing, ", "))
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
