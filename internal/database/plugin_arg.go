package database

// Purpose: refuse a non-plugin --plugin argument before any migration scan
// (P7-PLUG-20 fix, D-0602). migrationsDir joins the plugin name to the plugin
// directory, so `nself db migrate up --plugin ../victim` would scan and apply
// SQL from a sibling directory, and `--plugin notify.prev` the update backup's.
// Inputs: the --plugin value ("" means the project's own migrations).
// Outputs: nil, or an E401 error.
// Constraints: no I/O. The grammar is pluginname.Reason, the same check
// plugin.ValidateName runs at the command layer: one path segment, lowercase
// slug, not a backup directory and not a reserved name (bin, common, status).

import (
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/pluginname"
)

// validatePluginArg refuses a non-empty plugin argument that is not a plain,
// non-reserved plugin name.
func validatePluginArg(plugin string) error {
	if plugin == "" {
		return nil
	}
	if reason := pluginname.Reason(plugin); reason != "" {
		return errs.Newf("E401", "invalid plugin name %q: %s", plugin, reason).
			WithFix("use the plugin name as shown by 'nself plugin list', for example 'nself db migrate up --plugin notify'")
	}
	return nil
}
