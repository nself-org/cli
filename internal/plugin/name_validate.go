package plugin

// Purpose: ValidateName is the one check every plugin name must pass before it
// is joined to the plugin directory. Without it `nself plugin remove ../../.ssh`
// reached os.RemoveAll on a path outside the plugin directory (P7-PLUG-20).
// Inputs: a plugin name exactly as the caller received it (CLI arg, MCP/admin
// request, registry or bundle member name).
// Outputs: nil for a plain plugin slug, or an E401 (invalid usage) error naming
// the rule that failed.
// Constraints: no I/O, no normalisation. The shape is the one manifest.go
// enforces on installed plugins (lowercase letters, digits, hyphen) widened
// only by '_' and '.' inside the name, matching the db seed name rule, so an
// existing installed directory (for example "notify.prev") stays addressable.
// Callers must use the SAME string for the bundle ledger lookup and for the
// removal, so they validate first and never lower-case or trim afterwards.

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// maxPluginNameLen caps a name well above any real slug and below filesystem
// component limits.
const maxPluginNameLen = 64

// pluginSlugRe is the allowed shape: starts with a lowercase letter or digit,
// then lowercase letters, digits, '.', '_' or '-'.
var pluginSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// ValidateName reports whether name is a plain plugin slug that is safe to join
// to the plugin directory. It rejects the empty string, ".", "..", any path
// separator (slash or backslash, and the OS separator), absolute paths, a
// leading '-', NUL and other control characters, whitespace, a trailing '.',
// and anything filepath.Base would change.
func ValidateName(name string) error {
	reason := ""
	switch {
	case name == "":
		reason = "it is empty"
	case name == "." || name == "..":
		reason = "it is a directory reference"
	case len(name) > maxPluginNameLen:
		reason = "it is too long"
	case strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, filepath.Separator):
		reason = "it contains a path separator"
	case filepath.IsAbs(name) || filepath.VolumeName(name) != "":
		reason = "it is an absolute path"
	case strings.HasPrefix(name, "-"):
		reason = "it starts with '-'"
	case hasControlChar(name):
		reason = "it contains a control character"
	case strings.HasSuffix(name, "."):
		reason = "it ends with '.'"
	case filepath.Base(name) != name:
		reason = "it is not a single path element"
	case !pluginSlugRe.MatchString(name):
		reason = "plugin names use lowercase letters, digits, '-', '_' and '.', and start with a letter or digit"
	}
	if reason == "" {
		return nil
	}
	return errs.Newf("E401", "invalid plugin name %q: %s", name, reason).
		WithFix("use the plugin name as shown by 'nself plugin list', for example 'nself plugin remove notify'")
}

// hasControlChar reports whether s holds NUL, any other ASCII control
// character, or DEL.
func hasControlChar(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
