package pluginname

// Purpose: ValidateName is the one check every plugin name must pass before it
// is joined to the plugin directory. Without it `nself plugin remove ../../.ssh`
// reached os.RemoveAll on a path outside the plugin directory (P7-PLUG-20).
// Inputs: a plugin name exactly as the caller received it (CLI arg, MCP/admin
// request, registry or bundle member name).
// Outputs: nil for a plain plugin slug, or an E401 (invalid usage) error naming
// the rule that failed.
// Constraints: no I/O, no normalisation. The shape is exactly the one
// manifest.go enforces on installed plugin names (namePattern: lowercase
// letters, digits and hyphen, starting with a letter). No '.' or '_': a dotted
// backup directory such as "notify.prev" is not a plugin and must never be
// addressable by remove, and sanitizeSchemaName maps '-' and '_' to the same
// schema so '_' would make two names collide.
// Callers must use the SAME string for the bundle ledger lookup and for the
// removal, so they validate first and never lower-case or trim afterwards.

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// PGIdentifierMax is PostgreSQL's identifier limit in bytes (NAMEDATALEN-1). Longer
// identifiers are silently truncated, so two long names could share one schema.
const PGIdentifierMax = 63

// longestIdentAffix is the extra bytes around a plugin name in the longest
// identifier generated from it: "np_" + name + "_role".
const longestIdentAffix = len("np_") + len("_role")

// MaxLength is the longest accepted plugin name: the longest generated
// identifier (np_<name>_role) must fit in PGIdentifierMax bytes. The longest real
// plugin name in the tree is far below this.
const MaxLength = PGIdentifierMax - longestIdentAffix

// pluginSlugRe is the allowed shape, identical to namePattern in manifest.go:
// a lowercase letter, then lowercase letters, digits or '-'.
var pluginSlugRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// reservedNames are plugin-shaped names that already mean something else to the
// CLI, so no plugin may take them. Every entry is derived from code that creates
// or reads the object, and a test (TestReservedNamesCoverSharedSchemas) fails when
// a new shared np_ schema appears without a matching entry here.
//
// Direct children of the plugin directory that are NOT plugins:
//   - "bin": the shared command-binary directory (router.go pluginBinDir,
//     skipped by mount/discover.go). `plugin remove bin` would delete every
//     published binary.
//   - ".nself-state" (bundle ledger), ".install.lock", ".staging-thirdparty-*",
//     ".bundle-rollback-*" and "<name>.prev" backups all start with '.' or hold a
//     '.', so pluginSlugRe already refuses them; TestInfrastructureEntriesNeverSlugs
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
var reservedNames = map[string]string{
	"bin":    "it is the shared plugin command-binary directory",
	"common": "it maps to np_common, the shared ledger schema",
	"status": "it maps to np_status, the schema of the nself-status-page plugin",
}

// slugReason returns why name is not a plain single-element slug, or "".
func slugReason(name string) string {
	switch {
	case name == "":
		return "it is empty"
	case name == "." || name == "..":
		return "it is a directory reference"
	case len(name) > MaxLength:
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
	case !pluginSlugRe.MatchString(name):
		return "plugin names use lowercase letters, digits and '-', and start with a letter"
	}
	return ""
}

// ValidateName reports whether name is a plain plugin slug that is safe to join
// to the plugin directory and is not a reserved infrastructure name. It rejects
// the empty string, ".", "..", any path separator (slash or backslash, and the
// OS separator), absolute paths, a leading '-', NUL and other control
// characters, whitespace, and anything filepath.Base would change.
func ValidateName(name string) error {
	reason := slugReason(name)
	if reason == "" {
		reason = reservedNames[name]
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

// ValidateCommandName reports whether cmd is a plain command name: the part after
// "nself-" in a published binary, and the first argument of the command proxy.
// Same shape as a plugin slug, but reserved plugin names are allowed.
func ValidateCommandName(cmd string) error {
	if reason := slugReason(cmd); reason != "" {
		return errs.Newf("E401", "invalid command name %q: %s", cmd, reason).
			WithFix("command names are lowercase letters, digits and '-'")
	}
	return nil
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
