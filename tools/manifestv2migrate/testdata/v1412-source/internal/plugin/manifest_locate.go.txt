package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Locating an installed plugin's manifest.
//
// Split out of cli_binary.go on the file-size ratchet, which picked a fair
// boundary: that file is about publishing and removing command binaries, and
// this is about finding the manifest that describes them.
//
// The nesting handled here is the whole reason this is more than one line —
// release tarballs carry a leading directory and extraction keeps it.

// readPluginManifest loads plugin.json from an installed plugin directory.
// Returns nil when it cannot be read; callers treat that as "assume the
// nself-<name> convention" rather than as a failure, because a plugin with a
// corrupt manifest must still be removable.
func readPluginManifest(destDir string) *PluginManifest {
	for _, p := range manifestCandidates(destDir) {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var m PluginManifest
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		return &m
	}
	return nil
}

// manifestCandidates lists where an installed plugin's manifest may sit.
//
// Release tarballs carry a leading directory — the source archive is
// `free/<name>/...` and the platform archive is `<name>/...` — and extraction
// keeps it, so an installed plugin is nested one or two levels below destDir
// rather than sitting at its root. readPluginManifest only ever looked at the
// root, so it returned nil for every plugin installed from a release, and every
// caller silently took its nil fallback instead. That is why removing a plugin
// tried to drop a schema it did not have.
//
// findExtractedBinary already copes with the same nesting by walking; this does
// the same thing with a bounded search rather than a full walk, since a
// manifest is always near the top.
func manifestCandidates(destDir string) []string {
	out := []string{filepath.Join(destDir, "plugin.json")}

	entries, err := os.ReadDir(destDir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(destDir, e.Name())
		out = append(out, filepath.Join(sub, "plugin.json"))

		// One more level, for the `free/<name>/` shape of the source archive.
		inner, err := os.ReadDir(sub)
		if err != nil {
			continue
		}
		for _, ie := range inner {
			if ie.IsDir() {
				out = append(out, filepath.Join(sub, ie.Name(), "plugin.json"))
			}
		}
	}
	return out
}

// IsCommandInstalled reports whether a plugin providing the named command is
// present, i.e. whether ProxyCommand would find a binary for it.
//
// Used by the CLI to decide whether a relocated command still needs its
// "moved to a plugin" notice. Once the plugin is installed the old spelling is
// the supported spelling, and repeating the install hint is noise.
func IsCommandInstalled(cmdName string) bool {
	candidate := PublishedBinaryPath("nself-" + cmdName)
	info, err := os.Stat(candidate)
	return err == nil && !info.IsDir()
}

// pluginOwnsTables reports whether a plugin declares any database tables.
//
// A plugin with none needs no schema, and every CLI-R11 extraction produces
// exactly that shape: a Go binary that adds a command, with "tables": [] in its
// manifest. Running the schema step for those meant `nself install <cmd>`
// required Docker and a running Postgres to create an empty schema, so a
// command-line tool could not be installed on a machine without a stack.
//
// A nil manifest is treated as owning tables: it means the manifest could not
// be read, and skipping schema creation on a guess is the more damaging
// mistake of the two.
func pluginOwnsTables(m *PluginManifest) bool {
	if m == nil {
		return true
	}
	return len(m.Tables) > 0
}
