package commands

import (
	"os"

	"github.com/nself-org/cli/internal/config"
)

// loadAutoInstallConfig lets project settings load without allowing the project
// to redirect the bootstrap registry, cache home, or plugin installation path.
func loadAutoInstallConfig() (*config.Config, error) {
	type previous struct {
		value   string
		present bool
	}
	saved := make(map[string]previous)
	for _, key := range []string{"NSELF_PLUGIN_REGISTRY", "HOME", "NSELF_PLUGIN_DIR"} {
		value, present := os.LookupEnv(key)
		saved[key] = previous{value, present}
	}
	defer func() {
		for key, old := range saved {
			if old.present {
				_ = os.Setenv(key, old.value)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	}()
	return loadConfig()
}
