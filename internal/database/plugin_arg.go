package database

// Purpose: refuse a path-shaped --plugin argument before any migration scan
// (P7-PLUG-20 fix). migrationsDir joins the plugin name to the plugin
// directory, so `nself db migrate up --plugin ../victim` would scan and apply
// SQL from a sibling directory.
// Inputs: the --plugin value ("" means the project's own migrations).
// Outputs: nil, or an E401 error.
// Constraints: no I/O. This layer only guarantees the name is one path segment.
// The full plugin-name grammar (plugin.ValidateName) runs at the command layer
// (validatePluginFlag); importing internal/plugin here would add a same-layer
// L2 edge that layers.yaml does not allow.

import (
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// validatePluginArg refuses a non-empty plugin argument that is not exactly one
// path segment under the plugin directory.
func validatePluginArg(plugin string) error {
	if plugin == "" {
		return nil
	}
	if plugin == "." || plugin == ".." || strings.ContainsAny(plugin, "/\\\x00") || filepath.VolumeName(plugin) != "" {
		return errs.Newf("E401", "invalid plugin name %q: must be a single plugin name, not a path", plugin)
	}
	return nil
}
