package config

// plugin_dir.go — the one plugin directory rule (D-0573).
//
// Purpose: every reader of the global plugin directory resolves it the same
// way: NSELF_PLUGIN_DIR (or the configured PLUGIN_DIR), else ~/.nself/plugins,
// with a leading "~" expanded. Before this file compose discovery, the plugin
// migrations, the nginx injection and the plugin commands each carried their
// own copy, and only the nginx one expanded "~" (the D-0478 class of drift).
// Inputs: a configured dir (may be empty or start with "~") and the
// environment.
// Outputs: the directory to read plugins from.
// Constraints: the "~" expansion is v1.5 only (D-0634): v1.4.13 used PLUGIN_DIR
// as written, and expanding it in a patch would publish every installed
// plugin's nginx snippet on the first rebuild after an upgrade.

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/compat"
)

// pluginDirEnv is the environment variable that overrides the plugin dir.
const pluginDirEnv = "NSELF_PLUGIN_DIR"

// DefaultPluginDir returns the global plugin installation directory:
// $NSELF_PLUGIN_DIR when set, else ~/.nself/plugins (/tmp/.nself/plugins when
// the home directory cannot be determined).
func DefaultPluginDir() string {
	if dir := strings.TrimSpace(os.Getenv(pluginDirEnv)); dir != "" {
		return ExpandPluginDir(dir)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join("/tmp", ".nself", "plugins")
	}
	return filepath.Join(home, ".nself", "plugins")
}

// ResolvePluginDir is a configured plugin dir (PLUGIN_DIR, which defaults to
// the literal "~/.nself/plugins") made usable: expanded, or DefaultPluginDir
// when nothing is configured.
func ResolvePluginDir(configured string) string {
	if dir := ExpandPluginDir(configured); dir != "" {
		return dir
	}
	return DefaultPluginDir()
}

// ExpandPluginDir is ExpandHomeDir under v1.5 only (D-0634). v1.4.13 used
// PLUGIN_DIR as written, so the default "~/.nself/plugins" never resolved: a
// default-config project got no plugin nginx snippets, and
// ${NSELF_PLUGIN_DIR} rendered as "~/.nself/plugins". Expanding it in a patch
// would publish every installed plugin at <plugin>.<base> on the first
// rebuild after an upgrade, and fail the build on a snippet the user had
// copied into nginx/sites by hand, so v1.4 keeps the old value.
func ExpandPluginDir(dir string) string {
	// compat.V15(D-0478): PLUGIN_DIR "~/..." used as written, no plugin snippets on default config -> "~" expanded, installed plugins' snippets injected
	if compat.V15() {
		return ExpandHomeDir(dir)
	}
	return dir
}

// ExpandHomeDir turns a leading "~" or "~/" into the user's home dir.
// config.ApplyDefaults sets PLUGIN_DIR to the literal "~/.nself/plugins",
// which os.Stat would otherwise read relative to the working dir, so no
// plugin snippet was ever found. Anything else is returned unchanged.
func ExpandHomeDir(dir string) string {
	if dir != "~" && !strings.HasPrefix(dir, "~/") {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return dir
	}
	return filepath.Join(home, strings.TrimPrefix(dir, "~"))
}
