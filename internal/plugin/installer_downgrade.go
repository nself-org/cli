package plugin

// installer_downgrade.go — refuse a registry release that is older than the installed plugin (D-0843).
//
// Purpose: a release signature proves origin, not recency, so a compromised registry could serve an older, validly signed, vulnerable release as the latest. In v1.5 an install or update whose registry version is below the installed one is refused before any download.
// Inputs: pluginDir, the plugin name, the registry version about to be installed.
// Outputs: nil (proceed, possibly after a warning) or a refusal that names --allow-downgrade.
// Constraints: the installed version is read from the plugin directory, or from name.prev when Update has parked it there (Update renames the directory before it calls Install). v1.4 keeps the warning only. NSELF_PLUGIN_ALLOW_DOWNGRADE=1 (set by --allow-downgrade) is the explicit opt-in. A registry-signed minimum version or timestamp needs contract plugins.release-signature v1.1 (D-0840); until then this local floor is the only guard, and it cannot protect a first install.

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/compat"
)

// allowDowngradeEnv is how the command layer threads --allow-downgrade down,
// like NSELF_PLUGIN_INSTALL_TIER.
const allowDowngradeEnv = "NSELF_PLUGIN_ALLOW_DOWNGRADE"

// installedPluginVersion returns the version of the copy on disk, preferring
// the live directory over the .prev backup, or "" when neither has a readable
// manifest.
func installedPluginVersion(pluginDir, name string) string {
	for _, dir := range []string{name, name + ".prev"} {
		if m, err := parseManifest(filepath.Join(pluginDir, dir, "plugin.json")); err == nil && m.Version != "" {
			return m.Version
		}
	}
	return ""
}

// checkDowngrade refuses (v1.5) or warns about (v1.4, or with
// --allow-downgrade) an install of newVersion over a higher installed version.
func checkDowngrade(pluginDir, name, newVersion string) error {
	installed := installedPluginVersion(pluginDir, name)
	if installed == "" || compareSemver(installed, newVersion) <= 0 {
		return nil
	}
	// compat.V15(D-0843): a downgrade only warns -> refused unless --allow-downgrade is given
	if compat.V15() && os.Getenv(allowDowngradeEnv) != "1" {
		return fmt.Errorf("plugin %q: the registry offers %s but %s is installed; refusing the downgrade (a signed release can still be an old, vulnerable one). Pass --allow-downgrade to install it anyway", name, newVersion, installed)
	}
	fmt.Fprintf(os.Stderr, "⚠ Downgrading %s from %s to %s — this may cause data loss\n", name, installed, newVersion)
	return nil
}
