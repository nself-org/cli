package plugin

// Purpose: ValidateName is the one check every plugin name must pass before it
// is joined to the plugin directory. Without it `nself plugin remove ../../.ssh`
// reached os.RemoveAll on a path outside the plugin directory (P7-PLUG-20).
// The grammar itself lives in internal/pluginname, so internal/database (a
// same-layer package) applies the identical rules (D-0602).
// Inputs: a plugin name exactly as the caller received it (CLI arg, MCP/admin
// request, registry or bundle member name).
// Outputs: nil for a plain plugin slug, or an E401 (invalid usage) error naming
// the rule that failed.
// Constraints: no I/O, no normalisation. A dotted backup directory such as
// "notify.prev" is not a plugin and must never be addressable by remove.
// Callers must use the SAME string for the bundle ledger lookup and for the
// removal, so they validate first and never lower-case or trim afterwards.

import (
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/pluginname"
)

// pgIdentMax is PostgreSQL's identifier limit in bytes (see pluginname.PgIdentMax).
const pgIdentMax = pluginname.PgIdentMax

// maxPluginNameLen is the longest accepted plugin name (see pluginname.MaxLen).
const maxPluginNameLen = pluginname.MaxLen

// pluginSlugRe is the allowed plugin name shape, identical to namePattern in
// manifest.go.
var pluginSlugRe = pluginname.SlugRe

// reservedNames are plugin-shaped names that already mean something else to the
// CLI (see pluginname.Reserved).
var reservedNames = pluginname.Reserved

// ValidateName reports whether name is a plain plugin slug that is safe to join
// to the plugin directory and is not a reserved infrastructure name.
func ValidateName(name string) error {
	if reason := pluginname.Reason(name); reason != "" {
		return errs.Newf("E401", "invalid plugin name %q: %s", name, reason).
			WithFix("use the plugin name as shown by 'nself plugin list', for example 'nself plugin remove notify'")
	}
	return nil
}

// ValidateCommandName reports whether cmd is a plain command name: the part after
// "nself-" in a published binary, and the first argument of the command proxy.
// Same shape as a plugin slug, but reserved plugin names are allowed.
func ValidateCommandName(cmd string) error {
	if reason := pluginname.SlugReason(cmd); reason != "" {
		return errs.Newf("E401", "invalid command name %q: %s", cmd, reason).
			WithFix("command names are lowercase letters, digits and '-'")
	}
	return nil
}

// ValidateBinaryName reports whether bin is a single plain path element that is
// safe to join to the plugin bin directory. A manifest's binaryName and
// cliCommands names come from the plugin author, so they get the same checks as
// a plugin name with the binary name alphabet (see pluginname.BinaryReason).
func ValidateBinaryName(bin string) error {
	reason := pluginname.BinaryReason(bin)
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
