// reverse_deps.go — which installed plugins depend on a plugin (D-0325).
//
// Purpose: let `nself bundle remove` keep a member that a plugin outside the
// bundle still needs, using the same scan `plugin remove` uses.
// Inputs: the plugin directory and a plugin name.
// Outputs: the directory names of the dependents.
// Constraints: read only; the name is validated before the scan.
package plugin

import (
	"encoding/json"
	"fmt"
)

// ReverseDependencies returns the directory names of installed plugins whose
// plugin.json lists name as a dependency (compared without regard to case).
// Directories without a manifest are skipped; unreadable or invalid manifests
// refuse the scan so callers cannot remove a dependency blindly. A missing plugin directory
// has no dependents. An invalid name is an error.
func ReverseDependencies(pluginDir, name string) ([]string, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	return checkReverseDependencies(pluginDir, name)
}

// Only dependency fields matter to removal. Legacy installs may lack metadata
// now required for installation; both native and grouped dependencies count.
func reverseManifestDependencies(data []byte) ([]string, error) {
	var doc *struct {
		Dependencies json.RawMessage `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("manifest must be an object")
	}
	if len(doc.Dependencies) == 0 {
		return nil, nil
	}
	var deps []string
	if err := json.Unmarshal(doc.Dependencies, &deps); err == nil {
		return deps, nil
	}
	var grouped groupedDeps
	if err := json.Unmarshal(doc.Dependencies, &grouped); err != nil {
		return nil, err
	}
	return grouped.Required, nil
}
