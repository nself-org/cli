// reverse_deps.go — which installed plugins depend on a plugin (D-0325).
//
// Purpose: let `nself bundle remove` keep a member that a plugin outside the
// bundle still needs, using the same scan `plugin remove` uses.
// Inputs: the plugin directory and a plugin name.
// Outputs: the manifest names of the dependents.
// Constraints: read only; the name is validated before the scan.
package plugin

// ReverseDependencies returns the manifest names of installed plugins whose
// plugin.json lists name as a dependency (compared without regard to case).
// Directories without a valid manifest are skipped. A missing plugin directory
// has no dependents. An invalid name is an error.
func ReverseDependencies(pluginDir, name string) ([]string, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	return checkReverseDependencies(pluginDir, name)
}
