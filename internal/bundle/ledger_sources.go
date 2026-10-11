package bundle

// ledger_sources.go — adapters between this package, the plugin directory and
// the ledger package (which imports neither).
//
// Purpose: give ledger.Store its bootstrap input (LedgerSources) and turn the
// plugins on disk after an install into ledger members (installedMembers).
// Inputs: the plugin directory ($NSELF_PLUGIN_DIR or ~/.nself/plugins).
// Outputs: ledger.Sources / []ledger.InstalledPlugin.
// Constraints: read only; a plugin without a valid plugin.json is not "on disk"
// (plugin.LoadManifestsFromDir skips it, as it skips the .nself-state directory).

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/plugin"
)

// LedgerSources returns the bootstrap source for the ledger stored in pluginDir:
// every bundle's membership plus the plugins installed there. A plugin
// directory that cannot be read is an error, never an empty Sources. So is a
// bundle catalog that cannot be loaded (P7-PLUG-20): with no catalog every
// plugin would bootstrap as explicit, which hides every bundle reference.
func LedgerSources(pluginDir string) func() (ledger.Sources, error) {
	return func() (ledger.Sources, error) {
		if err := EnsureLoaded(context.Background()); err != nil {
			return ledger.Sources{}, fmt.Errorf("loading the bundle catalog: %w", err)
		}
		var src ledger.Sources
		for _, b := range All() {
			src.Bundles = append(src.Bundles, ledger.BundleDef{Slug: b.Slug, Installable: b.IsInstallable(), Plugins: b.Plugins})
		}
		plugins, err := installedPlugins(pluginDir)
		if err != nil {
			return ledger.Sources{}, err
		}
		src.Plugins = plugins
		return src, nil
	}
}

// installedPlugins lists the plugins in pluginDir as ledger facts.
func installedPlugins(pluginDir string) ([]ledger.InstalledPlugin, error) {
	manifests, err := plugin.LoadManifestsFromDir(pluginDir)
	if err != nil {
		return nil, err
	}
	out := make([]ledger.InstalledPlugin, 0, len(manifests))
	for _, m := range manifests {
		tier := ledger.TierFree
		if m.Tier == "pro" || m.Tier == "paid" || m.Tier == ledger.TierLicensed || m.RequiresLicense || m.LicenseType == "pro" {
			tier = ledger.TierLicensed
		}
		out = append(out, ledger.InstalledPlugin{Name: m.Name, Version: m.Version, Tier: tier, Checksum: m.Checksum})
	}
	return out, nil
}

// installedMembers returns, in planned order, the planned plugins that are on
// disk in pluginDir (installed now, same version, or a higher version kept).
func installedMembers(pluginDir string, planned []string) ([]ledger.InstalledPlugin, error) {
	onDisk, err := installedPlugins(pluginDir)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]ledger.InstalledPlugin, len(onDisk))
	for _, p := range onDisk {
		byName[strings.ToLower(p.Name)] = p
	}
	var out []ledger.InstalledPlugin
	for _, name := range planned {
		if p, ok := byName[strings.ToLower(name)]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

// InstalledFacts returns the ledger facts (name, version, tier, checksum) of the
// named plugins that are on disk in pluginDir, from their plugin.json files. It
// is the exported form of installedMembers for callers outside this package
// (cmd/commands/plugin_install_ledger.go); a plugin that is not on disk is omitted.
func InstalledFacts(pluginDir string, names []string) ([]ledger.InstalledPlugin, error) {
	return installedMembers(pluginDir, names)
}

// LedgerStore returns the ledger store of pluginDir with its bootstrap source wired.
func LedgerStore(pluginDir string) *ledger.Store { return newLedgerStore(pluginDir) }
