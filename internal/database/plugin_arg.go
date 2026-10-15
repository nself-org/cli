package database

// Purpose: refuse a path-shaped --plugin argument before any migration scan
// (P7-PLUG-20 fix). migrationsDir joins the plugin name to the plugin
// directory, so `nself db migrate up --plugin ../victim` would scan and apply
// SQL from a sibling directory.
// Inputs: the --plugin value ("" means the project's own migrations).
// Outputs: nil, or an E401 error.
// Constraints: no I/O. The shared lower-layer validator enforces the complete
// plugin grammar without importing the plugin orchestration package.

import "github.com/nself-org/cli/internal/pluginname"

// validatePluginArg refuses a non-empty plugin argument that is not exactly one
// plugin slug, including reserved-name and PostgreSQL identifier limits.
func validatePluginArg(plugin string) error {
	if plugin == "" {
		return nil
	}
	return pluginname.ValidateName(plugin)
}
