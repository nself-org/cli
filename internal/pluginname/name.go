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
// Outputs: nil for a plain plugin slug, or an E401 (invalid usage) error naming
// the rule that failed.
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

	"github.com/nself-org/cli/internal/errs"
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

// Validate reports whether name is a plain plugin slug that is safe to join to
// the plugin directory and is not a reserved infrastructure name. It rejects
// the empty string, ".", "..", any path separator (slash or backslash, and the
// OS separator), absolute paths, a leading '-', NUL and other control
// characters, whitespace, and anything filepath.Base would change.
func Validate(name string) error {
	reason := SlugReason(name)
	if reason == "" {
		reason = Reserved[name]
		if reason != "" {
			reason = "it is reserved: " + reason
		}
	}
	if reason == "" {
		return nil
	}
	return errs.Newf("E401", "invalid plugin name %q: %s", name, reason).
		WithFix("use the plugin name as shown by 'nself plugin list', for example 'nself plugin remove notify'")
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
