package reconcile

// state_prune.go — drop legacy plugin-fragment keys from the generated-state
// record (debts D-0557, D-0318).
//
// Purpose: before D-0318 the build rewrote a plugin's compose fragment inside
// the shared plugin directory and recorded it as
// "@plugins/<name>/docker-compose.plugin.yml". The build now writes a
// project-local overlay (.nself/plugins/<name>/, a project file) and never
// touches the shared fragment, so an old "@plugins/" key can only go stale: a
// later plugin update changes the shared file and doctor would read the record
// as a hand edit forever.
// Inputs: the record being folded and the set of keys the build writes this run.
// Outputs: the record's Files without any "@plugins/" key the build is not
// writing.
// Constraints: pure on the record (no disk), so plan == apply stays exact.

import "strings"

// pruneLegacyPluginKeys deletes every "@plugins/" key except those in keep.
func (s *GeneratedState) pruneLegacyPluginKeys(keep map[string]bool) {
	for key := range s.Files {
		if strings.HasPrefix(key, PluginsPrefix) && !keep[key] {
			delete(s.Files, key)
		}
	}
}
