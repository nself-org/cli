package compose

// Purpose: keep the compose tests independent of the host's plugin directory.
// Inputs: none. Outputs: NSELF_PLUGIN_DIR points at a fresh empty directory for the whole
// test binary, so a minio plugin installed under ~/.nself/plugins cannot change a result
// (E138 tests would otherwise pass for the wrong reason, P7-PLUG-60).
// Constraints: tests that need a plugin install their own through the minioPluginInstalled
// seam or t.Setenv.

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "compose-plugins-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "compose tests: temp plugin dir:", err)
		os.Exit(1)
	}
	prev, had := os.LookupEnv("NSELF_PLUGIN_DIR")
	_ = os.Setenv("NSELF_PLUGIN_DIR", dir)
	code := m.Run()
	if had {
		_ = os.Setenv("NSELF_PLUGIN_DIR", prev)
	} else {
		_ = os.Unsetenv("NSELF_PLUGIN_DIR")
	}
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
