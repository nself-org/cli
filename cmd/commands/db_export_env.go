package commands

import (
	"os"
	"sort"

	"github.com/joho/godotenv"
	"github.com/nself-org/cli/internal/config"
)

// Purpose: list the config key names the project's own env cascade defines.
// Inputs: project directory. Outputs: sorted distinct names, never values.
// Constraints: the host shell is not consulted, so the list is the same from
// any shell; an unreadable or unparsable cascade file contributes nothing
// (config.Load already rejected malformed files before this runs).
func dbExportProjectKeys(dir string) []string {
	env, _ := config.ResolveEnv(dir)
	names := map[string]bool{}
	for _, f := range config.EnvCascade(dir, env, config.LegacyOrderActive()) {
		if !f.Exists {
			continue
		}
		data, err := os.ReadFile(f.Path)
		if err != nil {
			continue
		}
		m, err := godotenv.UnmarshalBytes(data)
		if err != nil {
			continue
		}
		for k := range m {
			names[k] = true
		}
	}
	keys := []string{}
	for k := range names {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
