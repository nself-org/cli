// Package pluginname holds the plugin-name grammar: the one check every plugin
// name passes before it is joined to the plugin directory.
//
// Purpose: internal/plugin and internal/database are same-layer (L2) packages
// with no import edge between them, yet both join a plugin name to the plugin
// directory. The grammar lives here, below both, so the database layer refuses
// a backup directory ("notify.prev") or a reserved name ("bin", "common",
// "status") exactly as the command layer does (D-0602, P7-PLUG-20).
// Inputs: a plugin name exactly as the caller received it (CLI arg, MCP/admin
// request, registry or bundle member name).
// Outputs: "" for a plain plugin slug, or the rule that failed. Callers wrap the
// reason in their own E401 (invalid usage) error: this package imports nothing
// from the module, so it adds no importer to internal/errs (fan-in ratchet).
// Constraints: no I/O, no normalisation. The shape is exactly the one
// plugin/manifest.go enforces on installed plugin names (lowercase letters,
// digits and hyphen, starting with a letter). No '.' or '_': a dotted backup
// directory such as "notify.prev" is not a plugin, and sanitizeSchemaName maps
// '-' and '_' to the same schema so '_' would make two names collide.
package pluginname

import (
	"path/filepath"
	"regexp"
	"strings"
)

// PgIdentMax is PostgreSQL's identifier limit in bytes (NAMEDATALEN-1). Longer
// identifiers are silently truncated, so two long names could share one schema.
const PgIdentMax = 63

// longestIdentAffix is the extra bytes around a plugin name in the longest
// identifier generated from it: "np_" + name + "_role".
const longestIdentAffix = len("np_") + len("_role")

// MaxLen is the longest accepted plugin name: the longest generated identifier
// (np_<name>_role) must fit in PgIdentMax bytes. The longest real plugin name
// in the tree is far below this.
const MaxLen = PgIdentMax - longestIdentAffix

// SlugRe is the allowed shape: a lowercase letter, then lowercase letters,
// digits or '-'.
var SlugRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Reserved are plugin-shaped names that already mean something else to the
// CLI, so no plugin may take them. Every entry is derived from code that creates
// or reads the object, and a test in internal/plugin
// (TestReservedNamesCoverSharedSchemas) fails when a new shared np_ schema
// appears without a matching entry here.
//
// Direct children of the plugin directory that are NOT plugins:
//   - "bin": the shared command-binary directory (router.go pluginBinDir,
//     skipped by mount/discover.go). `plugin remove bin` would delete every
//     published binary.
//   - ".nself-state" (bundle ledger), ".install.lock", ".staging-thirdparty-*",
//     ".bundle-rollback-*" and "<name>.prev" backups all start with '.' or hold a
//     '.', so SlugRe already refuses them; TestInfrastructureEntriesNeverSlugs
//     pins that.
//
// Schemas and roles a plugin name can map onto (np_<name with '-' as '_'>, and
// np_<name>_role):
//   - "common": schema np_common, the shared ledger schema (np_common.schema_versions,
//     np_common.plugin_schema_versions) created by database/migrate_ledger.go and
//     plugin/schema_reconcile.go. Removing plugin "common" would run
//     DROP SCHEMA "np_common" CASCADE.
//   - "status": schema np_status, created by the licensed nself-status-page plugin
//     (plugins-pro). A table-owning plugin named "status" removed without
//     --keep-data would run DROP SCHEMA "np_status" CASCADE and delete the
//     status-page data.
//
// np_plugins and np_ai_usage are tables, not schemas, so they do not collide.
var Reserved = map[string]string{
	"bin":    "it is the shared plugin command-binary directory",
	"common": "it maps to np_common, the shared ledger schema",
	"status": "it maps to np_status, the schema of the nself-status-page plugin",
}

// SlugReason returns why name is not a plain single-element slug, or "".
func SlugReason(name string) string {
	switch {
	case name == "":
		return "it is empty"
	case name == "." || name == "..":
		return "it is a directory reference"
	case len(name) > MaxLen:
		return "it is too long"
	case strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, filepath.Separator):
		return "it contains a path separator"
	case filepath.IsAbs(name) || filepath.VolumeName(name) != "":
		return "it is an absolute path"
	case strings.HasPrefix(name, "-"):
		return "it starts with '-'"
	case HasControlChar(name):
		return "it contains a control character"
	case filepath.Base(name) != name:
		return "it is not a single path element"
	case !SlugRe.MatchString(name):
		return "plugin names use lowercase letters, digits and '-', and start with a letter"
	}
	return ""
}

// BinaryRe is the allowed shape of a published command binary name (one path
// element). It is wider than a plugin slug only for '.', '_' and capitals.
var BinaryRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// BinaryReason returns why bin is not a single plain path element that is safe
// to join to the plugin bin directory, or "". Same checks as a plugin name (no
// separator, no dot reference, no control character, Base unchanged) with the
// BinaryRe alphabet.
func BinaryReason(bin string) string {
	switch {
	case bin == "":
		return "it is empty"
	case bin == "." || bin == "..":
		return "it is a directory reference"
	case len(bin) > MaxLen:
		return "it is too long"
	case strings.ContainsAny(bin, `/\`) || strings.ContainsRune(bin, filepath.Separator):
		return "it contains a path separator"
	case filepath.IsAbs(bin) || filepath.VolumeName(bin) != "":
		return "it is an absolute path"
	case HasControlChar(bin):
		return "it contains a control character"
	case filepath.Base(bin) != bin:
		return "it is not a single path element"
	case !BinaryRe.MatchString(bin):
		return "binary names use letters, digits, '-', '_' and '.', and start with a letter or digit"
	}
	return ""
}

// Reason returns why name is not a plain plugin slug that is safe to join to
// the plugin directory and not a reserved infrastructure name, or "" when it is.
// It rejects the empty string, ".", "..", any path separator (slash or
// backslash, and the OS separator), absolute paths, a leading '-', NUL and
// other control characters, whitespace, and anything filepath.Base would change.
func Reason(name string) string {
	if reason := SlugReason(name); reason != "" {
		return reason
	}
	if reason := Reserved[name]; reason != "" {
		return "it is reserved: " + reason
	}
	return ""
}

// HasControlChar reports whether s holds NUL, any other ASCII control
// character, or DEL.
func HasControlChar(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
