package build

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Purpose: verifies every installed Go plugin ships a Dockerfile with a
// HEALTHCHECK instruction. Split out of postvalidate.go (CLI-R12) as a pure
// move.
// Inputs: the global plugin directory.
// Outputs: a list of warning strings — never a hard error; missing manifests,
// non-Go plugins, and unreadable files are silently skipped.

// CheckGoPluginDockerfiles scans pluginDir for Go plugins and verifies each has
// a Dockerfile containing a HEALTHCHECK instruction. Returns a list of warning
// strings for plugins where the check fails. Never returns an error — missing
// Dockerfiles or unreadable files produce warnings, not hard failures.
func CheckGoPluginDockerfiles(pluginDir string) []string {
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return nil // no plugin dir, nothing to check
	}

	var warnings []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pluginName := entry.Name()

		// Read plugin.json to determine language.
		manifestPath := filepath.Join(pluginDir, pluginName, "plugin.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue // no manifest, skip
		}
		var manifest struct {
			Language string `json:"language"`
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue
		}

		// Only check Go plugins.
		lang := strings.ToLower(manifest.Language)
		if lang != "go" && lang != "golang" {
			continue
		}

		dockerfilePath := filepath.Join(pluginDir, pluginName, "Dockerfile")
		dockerfile, err := os.ReadFile(dockerfilePath)
		if err != nil {
			warnings = append(warnings,
				fmt.Sprintf("plugin %q: Dockerfile not found (Go plugins should include a HEALTHCHECK)", pluginName))
			continue
		}

		if !dockerfileHasHealthcheck(dockerfile) {
			warnings = append(warnings,
				fmt.Sprintf("plugin %q: Dockerfile is missing a HEALTHCHECK instruction", pluginName))
		}
	}
	return warnings
}

// dockerfileHasHealthcheck returns true if the dockerfile content contains a
// HEALTHCHECK instruction (case-insensitive, ignoring comments).
func dockerfileHasHealthcheck(content []byte) bool {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(strings.ToUpper(line), "HEALTHCHECK") {
			return true
		}
	}
	return false
}
