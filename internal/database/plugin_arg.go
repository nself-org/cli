package database

// Purpose: refuse a path-shaped --plugin argument before any migration scan
// (P7-PLUG-20 fix). migrationsDir joins the plugin name to the plugin
// directory, so `nself db migrate up --plugin ../victim` would scan and apply
// SQL from a sibling directory.
// Inputs: the --plugin value ("" means the project's own migrations).
// Outputs: nil, or the E401 error from plugin.ValidateName.
// Constraints: no I/O. The package-level `plugin` parameter name used across this
// package shadows the import, hence the alias.

import (
	pluginpkg "github.com/nself-org/cli/internal/plugin"
)

// validatePluginArg checks a non-empty plugin argument with plugin.ValidateName.
func validatePluginArg(plugin string) error {
	if plugin == "" {
		return nil
	}
	return pluginpkg.ValidateName(plugin)
}
