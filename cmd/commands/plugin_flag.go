package commands

// plugin_flag.go — the shared --plugin flag check for db commands.
//
// Purpose: refuse a path-shaped --plugin value before any directory is read
// (P7-PLUG-20). Split out of plugin_lifecycle.go for the 300-line cap.
// Inputs: the --plugin value ("" means the project's own migrations).
// Outputs: nil, or the E401 error from plugin.ValidateName.
// Constraints: no I/O.

import "github.com/nself-org/cli/internal/plugin"

// validatePluginFlag checks a non-empty --plugin value with plugin.ValidateName,
// so a path-shaped name is refused before any directory is read (P7-PLUG-20).
func validatePluginFlag(name string) error {
	if name == "" {
		return nil
	}
	return plugin.ValidateName(name)
}
