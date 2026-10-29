package commands

import (
	"os"
	"testing"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
)

// isolateConfigFixture prevents config.Load's exported values from earlier
// projects from enabling services or invalidating this fixture's credentials.
func isolateConfigFixture(t *testing.T) {
	t.Helper()
	t.Cleanup(build.SnapshotEnv())
	for _, key := range config.KnownEnvVars() {
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}
