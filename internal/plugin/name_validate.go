package plugin

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

// maxPluginNameLen caps a name well above any real slug and below filesystem
// component limits.
const maxPluginNameLen = 64

// pluginSlugRe is the allowed shape, identical to namePattern in manifest.go:
// a lowercase letter, then lowercase letters, digits or '-'.
var pluginSlugRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// binaryNameRe is the allowed shape of a published command binary name (one
// path element). It is wider than a plugin slug only for '.', '_' and capitals.
var binaryNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

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
	case filepath.Base(name) != name:
		reason = "it is not a single path element"
	case !pluginSlugRe.MatchString(name):
		reason = "plugin names use lowercase letters, digits and '-', and start with a letter"
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

// ValidateBinaryName reports whether bin is a single plain path element that is
// safe to join to the plugin bin directory. A manifest's binaryName and
// cliCommands names come from the plugin author, so they get the same checks as
// a plugin name (no separator, no dot reference, no control character, Base
// unchanged) with the binary name alphabet.
func ValidateBinaryName(bin string) error {
	reason := ""
	switch {
	case bin == "":
		reason = "it is empty"
	case bin == "." || bin == "..":
		reason = "it is a directory reference"
	case len(bin) > maxPluginNameLen:
		reason = "it is too long"
	case strings.ContainsAny(bin, `/\`) || strings.ContainsRune(bin, filepath.Separator):
		reason = "it contains a path separator"
	case filepath.IsAbs(bin) || filepath.VolumeName(bin) != "":
		reason = "it is an absolute path"
	case hasControlChar(bin):
		reason = "it contains a control character"
	case filepath.Base(bin) != bin:
		reason = "it is not a single path element"
	case !binaryNameRe.MatchString(bin):
		reason = "binary names use letters, digits, '-', '_' and '.', and start with a letter or digit"
	}
	if reason == "" {
		return nil
	}
	return errs.Newf("E401", "invalid plugin binary name %q: %s", bin, reason).
		WithFix("fix binaryName and cliCommands in the plugin manifest: each must be one plain file name")
}

// safePublishedBinaryPath returns where bin is published, after validating the
// name and proving the cleaned path stays inside the plugin bin directory
// (filepath.Rel), so a name can never reach a file outside it.
func safePublishedBinaryPath(bin string) (string, error) {
	if err := ValidateBinaryName(bin); err != nil {
		return "", err
	}
	dst := PublishedBinaryPath(bin)
	rel, err := filepath.Rel(pluginBinDir(), dst)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errs.Newf("E401", "plugin binary %q resolves outside the plugin bin directory", bin)
	}
	return dst, nil
}

// validateCLIBinaries validates every binary name the manifest would publish.
func validateCLIBinaries(name string, m *PluginManifest) error {
	for _, bin := range cliBinaryNames(name, m) {
		if _, err := safePublishedBinaryPath(bin); err != nil {
			return err
		}
	}
	return nil
}
