package build

// Purpose: catch two plugins whose nginx snippets map to one nginx/sites file
// name (<plugin>-<file>), where the later write would silently replace the
// earlier one (D-0287). Split from plugin_nginx.go for the 300-line cap.
// Inputs: the owners map built by injectPluginNginxRoutesVia, the destination
// name, the plugin and its source file.
// Outputs: nil (v1.4 warns once) or an E055 error from v1.5.

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
)

// nginxDestOwner is the plugin and source file that produced a sites file.
type nginxDestOwner struct{ plugin, source string }

// checkNginxDestCollision records destName as written by pluginName from
// source, and reports when a different plugin already wrote that name: the
// later file would replace the earlier one and its route would vanish.
func checkNginxDestCollision(owners map[string]nginxDestOwner, destName, pluginName, source string) error {
	// Keyed lower-case: the sites dir can be case-insensitive (APFS), where
	// A-b.conf and a-B.conf are one file.
	key := strings.ToLower(destName)
	prev, seen := owners[key]
	if !seen || prev.plugin == pluginName {
		owners[key] = nginxDestOwner{plugin: pluginName, source: source}
		return nil
	}
	msg := fmt.Sprintf("plugins %q and %q both map to nginx/sites/%s (%s and %s)", prev.plugin, pluginName, destName, prev.source, source)
	// compat.V15(D-0287): two plugins mapping to one nginx/sites file overwrite silently (warning) -> E055 fails the build
	if compat.V15() {
		e := errs.New("E055", msg)
		e.Why = "Both plugins map to the same nginx/sites file name (<plugin>-<file>), so one would silently replace the other and its route vanish."
		e.Fix = "Rename one plugin's nginx conf file, or remove one of the two plugins."
		return e
	}
	warnOnce(msg + "; the later plugin's file replaces the earlier one (an error from v1.5)")
	owners[key] = nginxDestOwner{plugin: pluginName, source: source}
	return nil
}
