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

	nbuild "github.com/nself-org/cli/internal/build"
)

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

// pluginEntryExists reports whether <plugin dir>/<entry> is still on disk.
// Only a not-exist result counts as gone.
func pluginEntryExists(entry string) bool {
	_, err := os.Lstat(filepath.Join(nbuild.DefaultPluginDir(), entry))
	return !errors.Is(err, fs.ErrNotExist)
}
