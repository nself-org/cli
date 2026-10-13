package bundle

// ledger_sources.go — adapters between this package, the plugin directory and
// the ledger package (which imports neither).
//
// Purpose: give ledger.Store its bootstrap input (LedgerSources) and turn the
// plugins on disk after an install into ledger members (installedMembers).
// Inputs: the plugin directory ($NSELF_PLUGIN_DIR or ~/.nself/plugins).
// Outputs: ledger.Sources / []ledger.InstalledPlugin.
// Constraints: read only; installedMembers reads only manifests that pass
// plugin.LoadManifest (plugin.LoadManifestsFromDir skips the rest, as it skips
// the .nself-state directory). The bootstrap scan is in installer_helpers_ledger.go.

import (
	"strings"

	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/plugin"
)

// LedgerSources returns the bootstrap source for the ledger stored in pluginDir.
// It is bootstrapSources (installer_helpers_ledger.go): membership from the
// full loaded catalogue (Names, so a bundle outside DisplayOrder counts) and
// one strict scan of the plugin directory. A plugin directory that cannot be
// read is an error, never an empty Sources. So is a bundle catalog that cannot
// be loaded (P7-PLUG-20): with no catalog every plugin would bootstrap as
// explicit, which hides every bundle reference. A plugin.json that cannot be
// read lands in Sources.Unreadable; the read-only Load turns that into a
// warning, so `bundle list` keeps working with one bad manifest.
func LedgerSources(pluginDir string) func() (ledger.Sources, error) {
	return bootstrapSources(pluginDir)
}

// installedPlugins lists the plugins in pluginDir as ledger facts.
func installedPlugins(pluginDir string) ([]ledger.InstalledPlugin, error) {
	manifests, err := plugin.LoadManifestsFromDir(pluginDir)
	if err != nil {
		return nil, err
	}
	out := make([]ledger.InstalledPlugin, 0, len(manifests))
	for _, m := range manifests {
		out = append(out, ledgerFact(m))
	}
	return out, nil
}

// ledgerFact maps a manifest to the ledger fact for its plugin: the tier is
// licensed for a pro/paid/licensed tier, a licence requirement or a pro licence type.
func ledgerFact(m *plugin.PluginManifest) ledger.InstalledPlugin {
	tier := ledger.TierFree
	if m.Tier == "pro" || m.Tier == "paid" || m.Tier == ledger.TierLicensed || m.RequiresLicense || m.LicenseType == "pro" {
		tier = ledger.TierLicensed
	}
	return ledger.InstalledPlugin{Name: m.Name, Version: m.Version, Tier: tier, Checksum: m.Checksum}
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
