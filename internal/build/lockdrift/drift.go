// Package lockdrift compares installed plugin manifests with the bundle ledger.
package lockdrift

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/plugin"
)

// Sources adapts internal/bundle and the plugin directory to the ledger's
// bootstrap input (the ledger imports neither).
func Sources(pluginDir string) func() (ledger.Sources, error) {
	return func() (ledger.Sources, error) {
		var src ledger.Sources
		for _, b := range bundle.All() {
			src.Bundles = append(src.Bundles, ledger.BundleDef{Slug: b.Slug, Installable: b.IsInstallable(), Plugins: b.Plugins})
		}
		manifests, err := plugin.LoadManifestsFromDir(pluginDir)
		if err != nil {
			return ledger.Sources{}, err
		}
		for _, m := range manifests {
			tier := ledger.TierFree
			if m.Tier == "pro" || m.Tier == "paid" || m.Tier == ledger.TierLicensed || m.RequiresLicense || m.LicenseType == "pro" {
				tier = ledger.TierLicensed
			}
			src.Plugins = append(src.Plugins, ledger.InstalledPlugin{Name: m.Name, Version: m.Version, Tier: tier, Checksum: m.Checksum})
		}
		return src, nil
	}
}

// PluginLedgerDrift compares the bundle ledger's plugin records against the
// actual plugin manifests on disk (P7-PLUG-19). A plugin dir updated by copy
// or git pull while the ledger is stale would otherwise pass --frozen silently.
// Returns a list of human-readable drift descriptions; empty means no drift.
func PluginLedgerDrift(pluginDir string) ([]string, error) {
	store := ledger.NewStore(pluginDir)
	store.Sources = Sources(pluginDir)
	led, persisted, err := store.Load()
	if err != nil {
		return nil, fmt.Errorf("reading the bundle ledger: %w", err)
	}
	if !persisted {
		// No ledger file yet; bootstrap just ran from the same manifests we
		// would compare, so there is no drift to report.
		return nil, nil
	}
	manifests, err := plugin.LoadManifestsFromDir(pluginDir)
	if err != nil {
		return nil, fmt.Errorf("reading plugin manifests: %w", err)
	}
	actual := map[string]ledger.InstalledPlugin{}
	for _, m := range manifests {
		tier := ledger.TierFree
		if m.Tier == "pro" || m.Tier == "paid" || m.Tier == ledger.TierLicensed || m.RequiresLicense || m.LicenseType == "pro" {
			tier = ledger.TierLicensed
		}
		actual[strings.ToLower(strings.TrimSpace(m.Name))] = ledger.InstalledPlugin{
			Name:     m.Name,
			Version:  m.Version,
			Tier:     tier,
			Checksum: m.Checksum,
		}
	}
	var drift []string
	for slug, rec := range led.Plugins {
		a, ok := actual[slug]
		if !ok {
			drift = append(drift, fmt.Sprintf("plugin %s: ledger records version %s but plugin dir has no manifest", slug, rec.Version))
			continue
		}
		if rec.Version != a.Version {
			drift = append(drift, fmt.Sprintf("plugin %s: ledger version %s != dir version %s", slug, rec.Version, a.Version))
		}
		if rec.Checksum != "" && a.Checksum != "" && rec.Checksum != a.Checksum {
			drift = append(drift, fmt.Sprintf("plugin %s: ledger checksum %s != dir checksum %s", slug, rec.Checksum, a.Checksum))
		}
	}
	for slug := range actual {
		if _, ok := led.Plugins[slug]; !ok {
			drift = append(drift, fmt.Sprintf("plugin %s: present in plugin dir but not in ledger", slug))
		}
	}
	sort.Strings(drift)
	return drift, nil
}
