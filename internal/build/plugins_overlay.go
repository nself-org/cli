package build

import (
	"fmt"
	"path/filepath"
)

// Purpose: isolate compose normalizations from the shared plugin installation.
// Inputs: the project Sink, workdir, plugin name and normalized fragment bytes.
// Outputs: the absolute overlay path used in the compose manifest, or an error.
// Constraints: always regenerate from installed bytes, including mode switches;
// build contexts still point to installed sources through NSELF_PLUGIN_DIR.
func writePluginComposeOverlay(sink Sink, workdir, name string, data []byte) (string, error) {
	path, err := filepath.Abs(filepath.Join(workdir, ".nself", "plugins", name, pluginComposeFilename))
	if err != nil {
		return "", fmt.Errorf("resolving plugin compose overlay: %w", err)
	}
	if err := sink.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("creating plugin compose overlay directory: %w", err)
	}
	if err := sink.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("writing plugin compose overlay: %w", err)
	}
	return path, nil
}
