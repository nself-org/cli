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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/ui"
)

// composeManifestFile is the path relative to workdir where the ordered list
// of compose files (base + plugins) is recorded. Start/stop/restart commands
// read this file to build the `-f` flag list for docker compose.
const composeManifestFile = ".nself/compose-files.txt"

// pluginComposeFilename is the well-known name for a plugin's Docker Compose
// fragment. Plugins that only contribute background processes (no containers)
// will not have this file and are silently skipped (a plugin that declares a
// compose service and lacks it is E128 in v1.5, see missingFragmentIsError).
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
	return discoverPluginComposeFilesFx(writeEffects{}, newDiskSink(workdir), workdir, pluginDir)
}

// discoverPluginComposeFilesFx is DiscoverPluginComposeFiles with the in-place
// fragment normalisation routed through fx: plan mode records one
// plugin-fragment effect per rewritten fragment and leaves the file alone.
func discoverPluginComposeFilesFx(fx Effects, sink Sink, workdir, pluginDir string) ([]string, error) {
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
			if missingFragmentIsError(pluginDir, entry.Name()) {
				// compat.V15(P7-PLUG-17): a compose plugin without a fragment is skipped silently -> the build fails with E128 (v1.4 warns)
				if compat.V15() {
					return nil, errs.Newf("E128", "plugin %q declares a compose service but has no %s", entry.Name(), pluginComposeFilename)
				}
				ui.Warn(fmt.Sprintf("plugin %q declares a compose service but has no %s; it is left out of the stack (an error from v1.5)", entry.Name(), pluginComposeFilename))
			}
			continue
		}

		// Normalize stale Dockerfile references in place. Installed compose
		// files may predate the Rust→Go migration and reference "Dockerfile.go"
		// or "Dockerfile.golang" that no longer exist. Fix them so `nself build`
		// always produces a working docker-compose manifest without requiring
		// manual docker-compose.override.yml edits.
		if content, readErr := os.ReadFile(absPath); readErr == nil {
			normalized := normalizeComposeDockerfile(content, pluginDir, entry.Name())
			// Attach missing networks, write ${DOCKER_NETWORK:-x} as
			// ${DOCKER_NETWORK} and refuse a foreign external network, before
			// the alias pass so it sees the final network list.
			normalized, netErr := normalizeComposeNetworks(normalized, entry.Name())
			if netErr != nil {
				return nil, netErr
			}
			normalized = normalizeComposeNetworkAliases(normalized, entry.Name())
			// Rebuild nself/* images from source instead of pulling
			// never-published tags, and drop the obsolete version: key —
			// see plugins_image_to_build.go for the full rationale.
			normalized = normalizeComposeImageToBuild(normalized, pluginDir, entry.Name())
			// Rewrite ANY relative build.context (a bare "." or "./sub", a
			// source-repo-relative "${NSELF_PLUGIN_DIR}/../..", etc.) to the
			// canonical ${NSELF_PLUGIN_DIR}/<name> shape — the only form
			// that resolves correctly once merged as a non-first `-f` file
			// against docker compose. See plugins_build_context.go for the
			// full rationale (defect #10 / E2E golden path step 13).
			normalized = normalizeComposeBuildContext(normalized, pluginDir, entry.Name())
			normalized = normalizeComposeDropObsoleteVersion(normalized)
			// Inject the fixed core env vars (project identity, Postgres,
			// Hasura, PLUGIN_INTERNAL_SECRET/NOTIFY_INTERNAL_SECRET) every
			// plugin container needs but installed fragments never declared
			// — see plugins_core_env.go for the full rationale (E2E golden
			// path step 13, testproject_ai/testproject_mux).
			normalized = normalizeComposePluginCoreEnv(normalized, pluginDir, entry.Name())
			if !bytes.Equal(normalized, content) {
				// Write the corrected file back so the manifest references a valid compose.
				// The bytes go through the Sink in both modes (P7-LIVE-03) so a plan
				// carries and binds them; a real write error stays tolerated as
				// before, a deviation from a confirmed render does not.
				_ = fx.Do(EffectPluginFragment, absPath, "normalise plugin compose fragment in place", nil)
				if werr := sink.WriteFile(absPath, normalized, 0644); werr != nil {
					var dev *deviationError
					if errors.As(werr, &dev) {
						return nil, werr
					}
				}
			}
		}

		composePaths = append(composePaths, absPath)
	}

	return composePaths, nil
}

// missingFragmentIsError reports whether an installed plugin that ships no
// compose fragment is one that should have it: a v2 manifest with
// service.kind compose, or a v1 manifest with a Dockerfile and a port. A
// plugin whose manifest is absent or unreadable is not one (no signal).
func missingFragmentIsError(pluginDir, name string) bool {
	data, err := os.ReadFile(filepath.Join(pluginDir, name, "plugin.json"))
	if err != nil {
		return false
	}
	var m struct {
		Version json.RawMessage `json:"manifest_version"`
		Port    int             `json:"port"`
		Service *struct {
			Kind string `json:"kind"`
		} `json:"service"`
	}
	if json.Unmarshal(data, &m) != nil {
		return false
	}
	if strings.TrimSpace(string(m.Version)) == "2" {
		return m.Service != nil && m.Service.Kind == "compose"
	}
	if m.Port <= 0 {
		return false
	}
	_, err = os.Stat(filepath.Join(pluginDir, name, "Dockerfile"))
	return err == nil
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
// any "dockerfile:" directive that references a non-existent legacy file
// (left over from the Rust→Go migration) is corrected to point to
// "Dockerfile" when a canonical Dockerfile exists in the plugin dir. Returns
// the (possibly unchanged) content.
//
// Matching is line-based and exact-value (via composeBuildDockerfileRE, the
// same "dockerfile:" scalar matcher normalizeComposeBuildContext uses), never
// a substring/bytes.ReplaceAll. A prior version used
// bytes.ReplaceAll([]byte("dockerfile: Dockerfile.go"), ...) against the raw
// file, which also matched inside "dockerfile: Dockerfile.golang" — "Dockerfile.go"
// is a literal prefix of "Dockerfile.golang" — corrupting it to
// "dockerfile: Dockerfilelang" (E2E golden path step 13, claw plugin: docker
// failed with "open Dockerfilelang: no such file or directory"). mux, google,
// podcast and post ship the identical "Dockerfile.golang" shape and were
// silently corrupted the same way. Exact per-line value comparison against
// the legacy set cannot partial-match a longer name.
func normalizeComposeDockerfile(content []byte, pluginDir, pluginName string) []byte {
	canonical := canonicalDockerfile(pluginDir, pluginName)
	if canonical == "" {
		return content // no canonical Dockerfile found — leave as-is
	}

	// Legacy dockerfile names produced during the Rust→Go migration.
	legacy := map[string]bool{"Dockerfile.go": true, "Dockerfile.golang": true, "Dockerfile.rust": true}

	lines := strings.Split(string(content), "\n")
	changed := false
	for i, line := range lines {
		m := composeBuildDockerfileRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		indent, val := m[1], m[2]
		if !legacy[val] {
			continue // not one of the known legacy names — never touch
		}
		// Only replace when the referenced file does NOT actually exist, to
		// avoid clobbering plugins that legitimately ship multiple Dockerfiles
		// (e.g. mux/google/claw/post ship both Dockerfile and Dockerfile.golang
		// and intentionally build from the latter).
		oldPath := filepath.Join(pluginDir, pluginName, val)
		if _, err := os.Stat(oldPath); err == nil {
			continue // file exists — keep the reference as authored
		}
		lines[i] = indent + "dockerfile: " + canonical
		changed = true
	}
	if !changed {
		return content
	}
	return []byte(strings.Join(lines, "\n"))
}
