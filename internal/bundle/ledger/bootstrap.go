// Bootstrap: derive a first ledger from what is on disk.
//
// Purpose: a project that installed bundles before the ledger existed has no
// record of why its plugins are present. Bootstrap rebuilds that record from
// the installed plugin directories plus bundle membership (bundles.json).
// Inputs: Sources, built by the caller from internal/bundle and internal/plugin.
// Outputs: a valid Ledger.
// Constraints: a bundle counts as installed only when it is installable as a
// unit and EVERY member plugin is installed (a bundle install is all or
// nothing, with rollback). A plugin referenced by no installed bundle is
// explicit. Overlap is honoured: installed_by lists every installed bundle
// that contains the plugin. A plugin installed on its own AND later covered by
// a bundle cannot be told apart from the bundle install on disk; bootstrap
// treats it as bundle-owned, and P7-PLUG-19/20 record explicit installs from
// then on.
package ledger

import (
	"sort"
	"strings"
	"time"
)

// BundleDef is one bundle's membership.
type BundleDef struct {
	Slug string
	// Installable is false for structural bundles (task, nself-plus) that are
	// never installed as a unit; they are never recorded as installed.
	Installable bool
	Plugins     []string
}

// InstalledPlugin is one plugin found on disk.
type InstalledPlugin struct {
	Name     string
	Version  string
	Tier     string // free | licensed; anything else is treated as free
	Checksum string // lowercase hex sha256 or empty
}

// Sources is everything Bootstrap reads.
type Sources struct {
	Bundles []BundleDef
	Plugins []InstalledPlugin
	// Now stamps installed_at; nil means time.Now.
	Now func() time.Time
}

func (s Sources) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Bootstrap builds the first ledger from s.
func Bootstrap(s Sources) Ledger {
	l := New()
	stamp := s.now().Format(time.RFC3339)

	byName := map[string]InstalledPlugin{}
	for _, p := range s.Plugins {
		name := strings.ToLower(strings.TrimSpace(p.Name))
		if name != "" {
			byName[name] = p
		}
	}

	installedBy := map[string][]string{}
	for _, b := range s.Bundles {
		slug := strings.ToLower(b.Slug)
		if !b.Installable || len(b.Plugins) == 0 {
			continue
		}
		all := true
		for _, m := range b.Plugins {
			if _, ok := byName[strings.ToLower(m)]; !ok {
				all = false
				break
			}
		}
		if !all {
			continue
		}
		l.Bundles[slug] = BundleRecord{InstalledAt: stamp}
		for _, m := range b.Plugins {
			m = strings.ToLower(m)
			installedBy[m] = append(installedBy[m], slug)
		}
	}

	for name, p := range byName {
		by := installedBy[name]
		sort.Strings(by)
		rec := PluginRecord{InstalledBy: by, Explicit: len(by) == 0}
		fillFacts(&rec, p)
		l.Plugins[name] = rec
	}
	l.normalize()
	return l
}
