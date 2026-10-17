package build

// Purpose: catch two plugins whose nginx snippets map to one nginx/sites file
// name (<plugin>-<file>), where the later write would silently replace the
// earlier one (D-0287). Split from plugin_nginx.go for the 300-line cap.
// Inputs: the owners map built by injectPluginNginxRoutesVia, the destination
// name, the plugin and its source file.
// Outputs: nil (v1.4 warns once) or an E055 error from v1.5.
//
// Cross-build ownership (D-0491): every injected file carries an owner line
// "# nself-owner: <project>/<plugin>" after the generated marker. Before a
// write, an existing file that names a different owner, or that is a core
// route file (no owner line), is not replaced.

import (
	"fmt"
	"path/filepath"
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

// nginxOwnerPrefix starts the owner line stamped after the generated marker.
const nginxOwnerPrefix = "# nself-owner: "

// coreSiteFiles are core route files a plugin's <plugin>-<file> name can
// equal (storage + console.conf, api + docs.conf). The core writers do not
// stamp an owner, so these names are what mark a stamp-less file as core.
var coreSiteFiles = map[string]bool{"storage-console.conf": true, "api-docs.conf": true}

// stampNginxOwner puts the owner line on the line after the generated marker.
func stampNginxOwner(rendered, owner string) string {
	i := strings.Index(rendered, nginxGeneratedMarker)
	if i < 0 {
		return nginxOwnerPrefix + owner + "\n" + rendered
	}
	end := strings.IndexByte(rendered[i:], '\n')
	if end < 0 {
		return rendered + "\n" + nginxOwnerPrefix + owner + "\n"
	}
	at := i + end + 1
	return rendered[:at] + nginxOwnerPrefix + owner + "\n" + rendered[at:]
}

// nginxOwnerOf returns the owner named in content's first lines, or "".
func nginxOwnerOf(content string) string {
	for i, line := range strings.SplitN(content, "\n", 6) {
		if i == 5 {
			break
		}
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), nginxOwnerPrefix); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// checkNginxSiteOwner refuses to replace an existing sites file owned by
// someone else: a different project/plugin stamp (two projects sharing a
// fronting stack's sites dir), or a core route file. Unstamped files with a
// plugin-shaped name are legacy output and are upgraded. v1.4 warns once and
// lets the write proceed; v1.5 returns E055.
func checkNginxSiteOwner(sink Sink, sitesDir, destName, owner string) error {
	existing, err := sink.ReadFile(filepath.Join(sitesDir, destName))
	if err != nil {
		return nil // a missing (or unreadable) file has no owner to protect
	}
	prev := nginxOwnerOf(string(existing))
	if prev == owner || (prev == "" && !coreSiteFiles[strings.ToLower(destName)]) {
		return nil
	}
	who := "a core route file"
	if prev != "" {
		who = "owner " + prev
	}
	msg := fmt.Sprintf("nginx/sites/%s belongs to %s; plugin %s would replace it", destName, who, owner)
	// compat.V15(D-0491): replacing another owner's or a core nginx/sites file warns -> E055 fails the build
	if compat.V15() {
		e := errs.New("E055", msg)
		e.Why = "The file already in nginx/sites was written by a different project/plugin or by the core route writer, so replacing it would drop that route."
		e.Fix = "Rename the plugin's nginx conf file, remove the conflicting plugin, or give the projects distinct plugin sets."
		return e
	}
	warnOnce(msg + " (an error from v1.5)")
	return nil
}
