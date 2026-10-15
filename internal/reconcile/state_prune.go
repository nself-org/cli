package reconcile

// state_prune.go — drop a removed plugin's entries from the generated-state
// record (debt D-0557).
//
// Purpose: a plugin's compose fragment is recorded as
// "@plugins/<name>/docker-compose.plugin.yml". The build never lists a removed
// plugin's fragment in PlannedBuild.Removed (the plugin directory is gone, not
// a file the build deletes), so without this the key stayed in the record
// forever and a later reinstall read as hand-edited until the next build.
// Inputs: the record being folded and a test for whether a plugin entry (the
// first path segment under the plugin directory) still exists.
// Outputs: the record's Files without the "@plugins/" keys whose plugin entry
// is gone.
// Constraints: only a definite not-exist prunes; any other stat error keeps the
// key (an unreadable plugin directory is not proof the plugin was removed).
// A key the build is writing this run is never pruned. PlannedRecord runs the
// disk check once at plan time and the apply writes those exact bytes, so
// plan == apply stays exact.

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// withPluginKeys copies the record and migrates legacy absolute keys under
// dir to plugin keys, retaining their original hashes for hand-edit checks.
// Inputs: the resolved build/project plugin directory. Outputs: an owned copy.
// Constraints: canonical keys win; only definitely missing files are pruned;
// unrelated absolute paths and the caller's record remain untouched.
func (s *GeneratedState) withPluginKeys(dir string) *GeneratedState {
	next := &GeneratedState{SchemaVersion: s.SchemaVersion,
		Files: make(map[string]string, len(s.Files)), Host: append([]GeneratedHostEntry(nil), s.Host...)}
	for key, hash := range s.Files {
		next.Files[key] = hash
	}
	for key, hash := range s.Files {
		rel, legacy := strings.CutPrefix(key, AbsPrefix)
		if !legacy {
			continue
		}
		disk := string(filepath.Separator) + filepath.FromSlash(rel)
		canonical := displayPath(dir, disk)
		if !strings.HasPrefix(canonical, PluginsPrefix) {
			continue
		}
		delete(next.Files, key)
		if _, err := os.Lstat(disk); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if _, exists := next.Files[canonical]; !exists {
			next.Files[canonical] = hash
		}
	}
	return next
}

// pruneGonePlugins deletes every "@plugins/" key whose plugin entry is gone
// per exists, except keys in keep (the files this build writes).
func (s *GeneratedState) pruneGonePlugins(keep map[string]bool, exists func(entry string) bool) {
	for key := range s.Files {
		rel, ok := strings.CutPrefix(key, PluginsPrefix)
		if !ok || keep[key] {
			continue
		}
		entry, _, _ := strings.Cut(rel, "/")
		if entry == "" || entry == "." || entry == ".." {
			continue
		}
		if !exists(entry) {
			delete(s.Files, key)
		}
	}
}

// pluginEntryIn reports whether <dir>/<entry> is still on disk. Only a
// not-exist result counts as gone. dir is the plugin directory the build
// resolved (D-0607), never re-read from the process env.
func pluginEntryIn(dir string) func(entry string) bool {
	return func(entry string) bool {
		_, err := os.Lstat(filepath.Join(dir, entry))
		return !errors.Is(err, fs.ErrNotExist)
	}
}
