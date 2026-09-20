package build

// Purpose: plugin directory resolution and discovery of installed plugins'
// docker-compose fragments, plus in-place normalization of stale Dockerfile
// references left over from the Rust->Go migration.
// Inputs: workdir and the global plugin directory.
// Outputs: absolute compose-fragment paths, or normalized compose bytes.
// Constraints: the compose-manifest read/write and per-plugin env var
// computation moved to plugins_manifest.go, and the network-alias rewrite
// moved to plugins_network_alias.go — both split out (CLI-R12) as pure moves
// from this file. The image->build rewrite and obsolete version: strip live
// in plugins_image_to_build.go (see its header for rationale).

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// composeManifestFile is the path relative to workdir where the ordered list
// of compose files (base + plugins) is recorded. Start/stop/restart commands
// read this file to build the `-f` flag list for docker compose.
const composeManifestFile = ".nself/compose-files.txt"

// pluginComposeFilename is the well-known name for a plugin's Docker Compose
// fragment. Plugins that only contribute background processes (no containers)
// will not have this file and are silently skipped.
const pluginComposeFilename = "docker-compose.plugin.yml"

// DefaultPluginDir returns the default global plugin installation directory
// (~/.nself/plugins). The NSELF_PLUGIN_DIR environment variable overrides the
// default — used for per-project plugin sets, hermetic tests, and CI. Falls
// back to /tmp/.nself/plugins when the home directory cannot be determined.
func DefaultPluginDir() string {
	if dir := strings.TrimSpace(os.Getenv("NSELF_PLUGIN_DIR")); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/tmp", ".nself", "plugins")
	}
	return filepath.Join(home, ".nself", "plugins")
}

// DiscoverPluginComposeFiles scans pluginDir for installed plugins that
// contain a docker-compose.plugin.yml file. It returns absolute paths to
// each discovered compose file, sorted by plugin directory name for
// deterministic ordering. Plugins without a compose file are silently
// skipped (they are background-process plugins, not compose plugins).
func DiscoverPluginComposeFiles(workdir, pluginDir string) ([]string, error) {
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading plugin directory %s: %w", pluginDir, err)
	}

	var composePaths []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Skip disabled plugins (those with a .disabled marker file).
		disabledPath := filepath.Join(pluginDir, entry.Name(), ".disabled")
		if _, err := os.Stat(disabledPath); err == nil {
			continue
		}
		composePath := filepath.Join(pluginDir, entry.Name(), pluginComposeFilename)
		absPath, err := filepath.Abs(composePath)
		if err != nil {
			continue
		}
		if _, err := os.Stat(absPath); err != nil {
			continue
		}

		// Normalize stale Dockerfile references in place. Installed compose
		// files may predate the Rust→Go migration and reference "Dockerfile.go"
		// or "Dockerfile.golang" that no longer exist. Fix them so `nself build`
		// always produces a working docker-compose manifest without requiring
		// manual docker-compose.override.yml edits.
		if content, readErr := os.ReadFile(absPath); readErr == nil {
			normalized := normalizeComposeDockerfile(content, pluginDir, entry.Name())
			normalized = normalizeComposeNetworkAliases(normalized, entry.Name())
			// Rebuild nself/* images from source instead of pulling
			// never-published tags, and drop the obsolete version: key —
			// see plugins_image_to_build.go for the full rationale.
			normalized = normalizeComposeImageToBuild(normalized, pluginDir, entry.Name())
			// Rewrite a build.context authored for the source-repo layout
			// (e.g. "${NSELF_PLUGIN_DIR}/../.." + "free/cron/Dockerfile")
			// to the installed-layout shape — see plugins_image_to_build.go
			// for the full rationale (defect #10 / E2E golden path step 13).
			normalized = normalizeComposeBuildContext(normalized, pluginDir, entry.Name())
			normalized = normalizeComposeDropObsoleteVersion(normalized)
			if !bytes.Equal(normalized, content) {
				// Write the corrected file back so the manifest references a valid compose.
				_ = os.WriteFile(absPath, normalized, 0644)
			}
		}

		composePaths = append(composePaths, absPath)
	}

	return composePaths, nil
}

// canonicalDockerfile returns the correct Dockerfile name for a plugin.
// Plugins that have been migrated from Rust to Go ship a single "Dockerfile"
// (Go multi-stage). Legacy names "Dockerfile.go" and "Dockerfile.golang" were
// used during the transition period. This function normalises to "Dockerfile"
// whenever that file actually exists in the plugin directory, regardless of
// what the installed docker-compose.plugin.yml references.
func canonicalDockerfile(pluginDir, pluginName string) string {
	canonical := filepath.Join(pluginDir, pluginName, "Dockerfile")
	if _, err := os.Stat(canonical); err == nil {
		return "Dockerfile"
	}
	return ""
}

// normalizeComposeDockerfile rewrites a plugin compose YAML in-memory so that
// any "dockerfile:" directive that references a non-existent file is corrected
// to point to "Dockerfile" when a canonical Dockerfile exists in the plugin dir.
// Returns the (possibly unchanged) content.
func normalizeComposeDockerfile(content []byte, pluginDir, pluginName string) []byte {
	canonical := canonicalDockerfile(pluginDir, pluginName)
	if canonical == "" {
		return content // no canonical Dockerfile found — leave as-is
	}

	// Legacy dockerfile names produced during the Rust→Go migration.
	legacy := []string{"Dockerfile.go", "Dockerfile.golang", "Dockerfile.rust"}

	for _, old := range legacy {
		// Only replace when the referenced file does NOT actually exist, to
		// avoid clobbering plugins that legitimately ship multiple Dockerfiles.
		oldPath := filepath.Join(pluginDir, pluginName, old)
		if _, err := os.Stat(oldPath); err == nil {
			continue // file exists — keep the reference as authored
		}
		needle := []byte("dockerfile: " + old)
		replacement := []byte("dockerfile: " + canonical)
		content = bytes.ReplaceAll(content, needle, replacement)
	}
	return content
}
