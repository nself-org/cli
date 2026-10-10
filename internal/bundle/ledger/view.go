// View: the per-bundle slice of the ledger that `nself bundle list --json`
// prints under the additive `ledger` key of each row.
//
// Purpose: show a bundle's recorded state without exposing the whole file.
// Inputs: a Ledger, a bundle slug and that bundle's member plugin slugs.
// Outputs: a BundleView whose plugin list is sorted and never null.
// Constraints: pure; no file access.
package ledger

import (
	"sort"
	"strings"
)

// PluginView is one member plugin as the ledger records it.
type PluginView struct {
	Slug        string   `json:"slug"`
	InstalledBy []string `json:"installed_by"`
	Explicit    bool     `json:"explicit"`
	Tier        string   `json:"tier"`
	Version     string   `json:"version"`
	Checksum    string   `json:"checksum"`
}

// BundleView is one bundle's ledger state.
type BundleView struct {
	Installed   bool         `json:"installed"`
	InstalledAt string       `json:"installed_at,omitempty"`
	Plugins     []PluginView `json:"plugins"`
}

// View returns the state of bundle slug and of those of its members the
// ledger records.
func (l Ledger) View(slug string, members []string) BundleView {
	slug = strings.ToLower(slug)
	v := BundleView{Plugins: []PluginView{}}
	if b, ok := l.Bundles[slug]; ok {
		v.Installed = true
		v.InstalledAt = b.InstalledAt
	}
	for _, m := range members {
		m = strings.ToLower(m)
		p, ok := l.Plugins[m]
		if !ok {
			continue
		}
		by := append([]string{}, p.InstalledBy...)
		v.Plugins = append(v.Plugins, PluginView{
			Slug: m, InstalledBy: by, Explicit: p.Explicit,
			Tier: p.Tier, Version: p.Version, Checksum: p.Checksum,
		})
	}
	sort.Slice(v.Plugins, func(i, j int) bool { return v.Plugins[i].Slug < v.Plugins[j].Slug })
	return v
}
