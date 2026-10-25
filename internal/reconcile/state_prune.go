package reconcile

// state_prune.go — drop legacy plugin-fragment keys from the generated-state
// record (debts D-0557, D-0318, D-0644).
//
// Purpose: before D-0318 the build rewrote a plugin's compose fragment inside
// the shared plugin directory and recorded it as
// "@plugins/<name>/docker-compose.plugin.yml". The build now writes a
// project-local overlay (.nself/plugins/<name>/, a project file) and never
// touches the shared fragment, so an old "@plugins/" key can only go stale: a
// later plugin update changes the shared file and doctor would read the record
// as a hand edit forever. A record written before cli#695 named the same file
// "@abs/<path>" when the shell's plugin directory differed from the build's;
// those keys go stale the same way (D-0644).
// Inputs: the record being folded, the set of keys the build writes this run
// and the build's plugin directory ("" means DefaultPluginDir).
// Outputs: the record's Files without any "@plugins/" key, nor any "@abs/" key
// under the plugin directory, that the build is not writing.
// Constraints: pure on the record (no disk), so plan == apply stays exact.

import (
	"path/filepath"
	"strings"
)

// pruneLegacyPluginKeys deletes every "@plugins/" key except those in keep, and
// every "@abs/" key that lies under pluginDir (a plugin fragment recorded under
// its absolute name before cli#695).
func (s *GeneratedState) pruneLegacyPluginKeys(keep map[string]bool, pluginDir string) {
	for key := range s.Files {
		if keep[key] {
			continue
		}
		if strings.HasPrefix(key, PluginsPrefix) || absPluginKey(key, pluginDir) {
			delete(s.Files, key)
		}
	}
}

// absPluginKey reports whether key is an "@abs/" key for a file under pluginDir.
func absPluginKey(key, pluginDir string) bool {
	if !strings.HasPrefix(key, AbsPrefix) {
		return false
	}
	abs := string(filepath.Separator) + filepath.FromSlash(strings.TrimPrefix(key, AbsPrefix))
	return strings.HasPrefix(displayPath(pluginDir, abs), PluginsPrefix)
}
