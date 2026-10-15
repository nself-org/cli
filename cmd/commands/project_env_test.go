package commands

import (
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// projectConfigKeys identifies config inputs exported by earlier fixture loads.
// Harness controls such as NSELF_V15 remain explicit inputs to each test.
func projectConfigKeys() map[string]bool {
	keys := make(map[string]bool)
	for _, key := range config.KnownEnvVars() {
		if key != "AI_AUTO_INSTALL" {
			keys[key] = true
		}
	}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		for _, prefix := range []string{"AUTH_", "HASURA_GRAPHQL_", "HASURA_EXTRA_", "REMOTE_SCHEMA_", "CS_", "FRONTEND_APP_", "INTERNAL_ROUTE_"} {
			if strings.HasPrefix(key, prefix) {
				keys[key] = true
			}
		}
	}
	return keys
}

// clearProjectConfigEnv gives an in-process fixture its own config defaults,
// restoring the caller's environment, including unset values, on cleanup.
func clearProjectConfigEnv(t *testing.T) {
	t.Helper()
	for key := range projectConfigKeys() {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}
