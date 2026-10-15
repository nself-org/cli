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
	"github.com/nself-org/cli/internal/pluginname"
)

// maxPluginNameLen preserves the shared PostgreSQL identifier limit.
const maxPluginNameLen = pluginname.MaxLength

// pgIdentMax keeps schema checks aligned with the shared name length limit.
const pgIdentMax = pluginname.PGIdentifierMax

// binaryNameRe permits the published command binary alphabet.
var binaryNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidateName validates a plugin slug using the shared database/plugin rules.
func ValidateName(name string) error {
	return pluginname.ValidateName(name)
}

// ValidateCommandName validates a command slug, allowing reserved plugin names.
func ValidateCommandName(cmd string) error {
	return pluginname.ValidateCommandName(cmd)
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
	case pluginname.HasControlChar(bin):
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
