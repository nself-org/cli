package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// entrypointSafeRe matches entrypoint paths composed solely of safe characters.
// Anchored at both ends to prevent partial matches.
var entrypointSafeRe = regexp.MustCompile(`^[a-zA-Z0-9_./-]+$`)

var pluginDevCmd = &cobra.Command{
	Use:   "dev <name>",
	Short: "Start a plugin in development mode with hot-reload",
	Long: `Start a plugin in development mode using air (preferred) or fswatch polling.

Wraps the plugin SDK's dev-watch.sh script with auto-link support so you can
iterate on plugin source and see changes reflected immediately.

  nself plugin dev myplugin              # auto-link + hot-reload
  nself plugin dev myplugin --no-link    # skip auto-link step
  nself plugin dev myplugin --debug      # attach dlv debugger`,
	Args: cobra.ExactArgs(1),
	RunE: runPluginDev,
}

func init() {
	pluginDevCmd.Flags().Bool("no-link", false, "Skip auto-linking the plugin before starting dev mode")
	pluginDevCmd.Flags().Bool("debug", false, "Attach dlv debugger (delegates to nself plugin debug)")
	pluginDevCmd.Flags().String("entrypoint", "./cmd", "Plugin entrypoint directory passed to dev-watch.sh")
	pluginCmd.AddCommand(pluginDevCmd)
}

func runPluginDev(cmd *cobra.Command, args []string) error {
	name := args[0]
	noLink, _ := cmd.Flags().GetBool("no-link")
	debug, _ := cmd.Flags().GetBool("debug")
	entrypoint, _ := cmd.Flags().GetString("entrypoint")

	// Resolve plugin directory — either cwd/<name> or an absolute path.
	pluginPath, err := resolvePluginPath(name)
	if err != nil {
		return err
	}

	uiInfof("Starting %s in development mode...", name)
	uiDimmedf("Plugin path: %s", pluginPath)

	// Auto-link unless --no-link is set.
	if !noLink {
		uiInfof("Auto-linking %s...", name)
		if linkErr := linkPlugin(pluginPath); linkErr != nil {
			uiWarnf("Auto-link failed (%v); continuing without link", linkErr)
		} else {
			ui.Success("Plugin linked.")
		}
	}

	// If --debug is requested, delegate to nself plugin debug.
	if debug {
		debugArgs := []string{"plugin", "debug", name}
		nself, err := os.Executable()
		if err != nil {
			nself = "nself"
		}
		uiInfof("Launching debugger via %s plugin debug %s", nself, name)
		debugCmd := exec.Command(nself, debugArgs...)
		debugCmd.Stdout = os.Stdout
		debugCmd.Stderr = os.Stderr
		return debugCmd.Run()
	}

	// Validate entrypoint against safe-path policy (defense-in-depth).
	// 1. Reject characters outside the safe set before any path resolution.
	if !entrypointSafeRe.MatchString(entrypoint) {
		return fmt.Errorf("plugin dev: entrypoint %q contains invalid characters (allowed: [a-zA-Z0-9_./-])", entrypoint)
	}
	// 2. filepath.Clean collapses ../ sequences; the HasPrefix check then
	//    prevents path traversal (e.g. --entrypoint ../../../../etc/passwd).
	cleanEntrypoint := filepath.Clean(entrypoint)
	if strings.Contains(cleanEntrypoint, "..") {
		return fmt.Errorf("plugin dev: entrypoint %q must not contain path traversal sequences", entrypoint)
	}
	resolvedEntrypoint := filepath.Join(pluginPath, cleanEntrypoint)
	if !strings.HasPrefix(resolvedEntrypoint, pluginPath+string(filepath.Separator)) &&
		resolvedEntrypoint != pluginPath {
		return fmt.Errorf("plugin dev: entrypoint %q must be within the plugin directory %q", entrypoint, pluginPath)
	}

	// Find dev-watch.sh in the SDK devkit or fall back to bundled copy.
	watchScript, err := findDevWatchScript()
	if err != nil {
		return fmt.Errorf("dev-watch.sh not found: %w\nInstall the plugin SDK or run `go install github.com/air-verse/air@latest`", err)
	}

	uiDimmedf("Using watch script: %s", watchScript)
	uiDimmedf("Entrypoint: %s", cleanEntrypoint)
	ui.Info("Watching for changes — press Ctrl+C to stop.")

	// Run dev-watch.sh from the plugin's source directory.
	devCmd := exec.Command("bash", watchScript, cleanEntrypoint)
	devCmd.Dir = pluginPath
	devCmd.Stdout = os.Stdout
	devCmd.Stderr = os.Stderr
	devCmd.Env = append(os.Environ(), "NSELF_PLUGIN_DEV=1")

	if err := devCmd.Run(); err != nil {
		// Non-zero exit is expected when user Ctrl+Cs (signal).
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 130 {
			return nil // SIGINT — clean exit
		}
		return fmt.Errorf("dev-watch exited: %w", err)
	}
	return nil
}

// resolvePluginPath returns the absolute path of the named plugin on disk.
// It checks: (1) cwd/<name>, (2) ~/.nself/plugin-links.json entry.
func resolvePluginPath(name string) (string, error) {
	if err := plugin.ValidateName(name); err != nil {
		return "", err
	}
	// Try current directory first.
	cwd, err := os.Getwd()
	if err == nil {
		candidate := filepath.Join(cwd, name)
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate, nil
		}
		// Also try cwd itself if it looks like a plugin directory.
		if _, err := os.Stat(filepath.Join(cwd, "plugin.yaml")); err == nil {
			return cwd, nil
		}
	}

	// Try the linked registry.
	links, err := loadPluginLinks()
	if err == nil {
		if path, ok := links[name]; ok {
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("plugin %q not found: run from plugin directory or link it first with `nself plugin link`", name)
}

// findDevWatchScript locates the dev-watch.sh script bundled with the SDK.
func findDevWatchScript() (string, error) {
	// Check well-known SDK paths relative to GOPATH and common module cache locations.
	candidates := []string{
		// Sibling repo (monorepo dev setup).
		filepath.Join(sdkRepoPath(), "devkit", "tools", "dev-watch.sh"),
		// GOPATH-installed SDK.
		filepath.Join(os.Getenv("GOPATH"), "pkg", "mod", "github.com", "nself-org", "plugin-sdk-go*", "devkit", "tools", "dev-watch.sh"),
	}

	for _, c := range candidates {
		// A candidate may carry an unexpanded glob segment (e.g. the
		// GOPATH module-cache path, which embeds the SDK's version suffix
		// as `plugin-sdk-go*`). os.Stat never matches a literal glob
		// pattern, so expand it explicitly and use the first match; a
		// plain path with no glob metacharacters passes through Glob
		// unchanged and behaves exactly like the old os.Stat check.
		matches, err := filepath.Glob(c)
		if err != nil || len(matches) == 0 {
			continue
		}
		if _, err := os.Stat(matches[0]); err == nil {
			return matches[0], nil
		}
	}

	// If not found, generate a minimal inline version so users don't hit a hard failure.
	return writeInlineDevWatchScript()
}

// sdkRepoPath returns the expected sibling repo path for monorepo setups.
func sdkRepoPath() string {
	// Navigate from this binary's location up to the project root.
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	// Walk up 3 levels from the binary (bin/nself → project root → ../plugins-pro).
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	return filepath.Join(projectRoot, "plugins-pro", "plugin-sdk-go")
}

// writeInlineDevWatchScript writes a minimal dev-watch.sh to a temp file and returns its path.
// This is the fallback when the SDK devkit is not installed.
func writeInlineDevWatchScript() (string, error) {
	script := `#!/usr/bin/env bash
set -euo pipefail
ENTRY="${1:-./cmd}"
if command -v air >/dev/null 2>&1; then
  if [ ! -f ./.air.toml ]; then
    cat >./.air.toml <<'TOML'
root = "."
tmp_dir = "tmp"
[build]
  cmd = "go build -o ./tmp/plugin ./cmd"
  bin = "tmp/plugin"
  delay = 500
  include_ext = ["go", "yaml", "yml"]
  exclude_dir = ["tmp", "vendor", ".git"]
[log]
  time = true
TOML
  fi
  exec air
fi
if command -v fswatch >/dev/null 2>&1; then
  printf 'dev-watch: air not installed; falling back to fswatch\n' >&2
  while true; do
    (go run "$ENTRY" &); pid=$!
    fswatch -1 -e ".*" -i "\\.go$" -i "\\.ya?ml$" . >/dev/null
    kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; sleep 0.5
  done
fi
printf 'dev-watch: neither air nor fswatch installed.\nInstall: go install github.com/air-verse/air@latest\n' >&2
exit 1
`
	tmpFile := filepath.Join(os.TempDir(), "nself-dev-watch.sh")
	if err := os.WriteFile(tmpFile, []byte(script), 0750); err != nil {
		return "", fmt.Errorf("writing inline dev-watch script: %w", err)
	}
	return tmpFile, nil
}
