package build

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// Purpose: the compose-files manifest (read/write, used by start/stop/restart
// to build docker compose's `-f` flag list) and per-plugin env var
// computation (PLUGIN_{DEP}_INTERNAL_URL wiring for Go plugin dependencies).
// Inputs: workdir, the base compose path or plugin file list, or the global
// plugin directory.
// Outputs: an error, an ordered []string of compose paths, or a map of env vars.
// Constraints: split out of plugins.go (CLI-R12) as a pure move; no behavior
// changed. Depends on composeManifestFile, defined in plugins.go.

// WriteComposeManifest writes .nself/compose-files.txt with one compose file
// path per line. The first line is always the base docker-compose.yml
// (absolute path). Subsequent lines are absolute paths to plugin compose
// files. The .nself/ directory is created if it does not exist.
func WriteComposeManifest(workdir string, baseCompose string, pluginFiles []string) error {
	absBase, err := filepath.Abs(baseCompose)
	if err != nil {
		return fmt.Errorf("resolving base compose path: %w", err)
	}

	manifestPath := filepath.Join(workdir, composeManifestFile)
	manifestDir := filepath.Dir(manifestPath)
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", manifestDir, err)
	}

	var lines []string
	lines = append(lines, absBase)
	lines = append(lines, pluginFiles...)

	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing compose manifest: %w", err)
	}

	return nil
}

// ReadComposeManifest reads .nself/compose-files.txt and returns the ordered
// list of compose file paths. If the manifest does not exist, it returns a
// single-element slice with "docker-compose.yml" (relative, base only) so
// callers always get a usable default. Lines pointing to files that no longer
// exist on disk (e.g. a plugin was uninstalled) are silently skipped.
func ReadComposeManifest(workdir string) ([]string, error) {
	manifestPath := filepath.Join(workdir, composeManifestFile)

	f, err := os.Open(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return withUserOverride(workdir, []string{"docker-compose.yml"}), nil
		}
		return nil, fmt.Errorf("opening compose manifest: %w", err)
	}
	defer func() { _ = f.Close() }()

	var paths []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Skip lines pointing to files that no longer exist.
		if _, err := os.Stat(line); err != nil {
			continue
		}
		paths = append(paths, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading compose manifest: %w", err)
	}

	// If the manifest was empty or all entries were stale, fall back to base.
	if len(paths) == 0 {
		return withUserOverride(workdir, []string{"docker-compose.yml"}), nil
	}

	return withUserOverride(workdir, paths), nil
}

// userComposeOverrideFile is the hand-written file Docker Compose merges
// automatically when it is invoked with no -f at all.
const userComposeOverrideFile = "docker-compose.override.yml"

// withUserOverride appends the user's docker-compose.override.yml when it sits
// next to the generated compose file.
//
// `docker compose` only auto-merges the override when invoked with NO -f flag.
// The CLI always passes -f (it has to: plugin fragments are separate files), so
// the override was silently inert on every project — the container came up with
// none of it applied and nothing said so. Reported by the ntask clean-fork
// self-host drill, 2026-08-24, where the override was the only place the
// functions service got its entrypoint, a writable rootfs and its SMTP/MinIO
// environment.
//
// It is appended LAST on purpose: Compose lets later -f files win, and a
// hand-written override must beat both the generated base and any plugin
// fragment, which is the whole reason it exists.
func withUserOverride(workdir string, paths []string) []string {
	for _, p := range paths {
		if filepath.Base(p) == userComposeOverrideFile {
			return paths // already listed; do not add it twice
		}
	}
	if _, err := os.Stat(filepath.Join(workdir, userComposeOverrideFile)); err != nil {
		return paths
	}
	return append(paths, userComposeOverrideFile)
}

// pluginManifestMinimal holds the fields we need from plugin.json during build.
type pluginManifestMinimal struct {
	Name                 string   `json:"name"`
	Port                 int      `json:"port"`
	Language             string   `json:"language"`
	Dependencies         []string `json:"dependencies"`
	OptionalDependencies []string `json:"optionalDependencies"`
}

// readPluginManifest reads the plugin.json from a plugin directory.
// Returns nil (not an error) if the file is absent or unparseable — callers
// should skip plugins whose manifests cannot be read.
func readPluginManifest(pluginDir, name string) *pluginManifestMinimal {
	path := filepath.Join(pluginDir, name, "plugin.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m pluginManifestMinimal
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return &m
}

// ComputePluginEnvVars returns environment variables needed by plugin compose
// files. These are written to .nself/compose.env and .env.computed so that
// `docker compose` can interpolate them in plugin compose fragments.
//
// Variables returned:
//   - NSELF_PLUGIN_DIR: absolute path to the global plugin directory
//   - PLUGIN_{NAME}_INTERNAL_URL: http://plugin-{name}:{port} for every
//     declared dependency (required + optional) of every installed plugin.
//     Only wired when the dependency plugin is also installed.
//   - the addPluginCoreEnvVars set (ENV, PROJECT_NAME, COMPOSE_PROJECT_NAME,
//     BASE_DOMAIN, POSTGRES_DB/USER, PLUGIN_INTERNAL_SECRET,
//     NOTIFY_INTERNAL_SECRET) — the project-specific values the ${VAR}
//     references normalizeComposePluginCoreEnv (plugins_core_env.go) injects
//     into every plugin fragment resolve to. cfg may be nil (existing
//     callers/tests that only need the two sets above); a nil cfg skips this
//     set entirely.
func ComputePluginEnvVars(workdir, pluginDir string, cfg *config.Config) map[string]string {
	vars := make(map[string]string)

	absPluginDir, err := filepath.Abs(pluginDir)
	if err != nil {
		absPluginDir = pluginDir
	}
	vars["NSELF_PLUGIN_DIR"] = absPluginDir
	addPluginCoreEnvVars(vars, cfg)

	// Build a port map for all installed plugins so dependency resolution is O(1).
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return vars
	}

	portByName := make(map[string]int)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		m := readPluginManifest(pluginDir, entry.Name())
		if m != nil && m.Port > 0 {
			portByName[m.Name] = m.Port
			if portByName[entry.Name()] == 0 {
				portByName[entry.Name()] = m.Port // also index by dir name
			}
		}
	}

	// For each installed Go plugin, inject PLUGIN_{DEP}_INTERNAL_URL for every
	// declared dependency that is also installed. Only Go plugins communicate
	// with sibling plugins over HTTP — non-Go plugins use other IPC mechanisms.
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		m := readPluginManifest(pluginDir, entry.Name())
		if m == nil || m.Language != "go" {
			continue
		}
		allDeps := append(m.Dependencies, m.OptionalDependencies...)
		for _, dep := range allDeps {
			port, ok := portByName[dep]
			if !ok || port == 0 {
				continue // dep not installed or has no port
			}
			key := "PLUGIN_" + strings.ToUpper(strings.ReplaceAll(dep, "-", "_")) + "_INTERNAL_URL"
			vars[key] = fmt.Sprintf("http://plugin-%s:%d", dep, port)
		}
	}

	return vars
}
