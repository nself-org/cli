package compose

// Purpose: keep the compose tests independent of the host's plugin directory.
// Inputs: none. Outputs: NSELF_PLUGIN_DIR points at a fresh empty directory for the whole
// test binary, so a minio plugin installed under ~/.nself/plugins cannot change a result
// (E138 tests would otherwise pass for the wrong reason, P7-PLUG-60).
// Constraints: tests that need a plugin install their own through the minioPluginInstalled
// seam or t.Setenv.

import (
	"fmt"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/config"
	"os"
	"testing"
)

// withTestAuthKey gives a v1.5 test config the AUTH_ENCRYPTION_KEY the build would have
// generated: v1.5 renders the current auth image, which is refused without it (D-0660).
// v1.4 configs stay as they are (their goldens carry no key).
func withTestAuthKey(cfg *config.Config) *config.Config {
	if compat.V15() {
		if cfg.Passthrough == nil {
			cfg.Passthrough = map[string]string{}
		}
		cfg.Passthrough[AuthEncryptionKeyEnv] = fakeAuthKey
	}
	return cfg
}

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
