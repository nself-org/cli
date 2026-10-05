package main

// counts.go: counts.json (contract:plugins.counts v1), computed from the two
// registries, never typed.
//
// Purpose: one rule for "how many plugins": a slug in both registries is one
// two-tier product and counts once; entries are never a headline number,
// advertised is.
// Inputs: the free and licensed model.Registry. Outputs: model.Counts.
// Constraints: installable comes from the entry's manifest-derived
// `installable: false`; slices are never null.

import (
	"sort"

	"github.com/nself-org/cli/tools/catalog/model"
)

// buildCounts computes the counts document.
func buildCounts(free, licensed *model.Registry, from model.GeneratedFrom) *model.Counts {
	fc, pc := tierCount(free), tierCount(licensed)
	shared := sortedKeys(sharedSlugs(free, licensed))
	installable := func(r *model.Registry, s string) bool {
		e := r.Plugins[s]
		return e.Installable == nil || *e.Installable
	}
	total := fc.Installable + pc.Installable
	for _, s := range shared {
		total -= boolInt(installable(free, s)) + boolInt(installable(licensed, s)) - boolInt(installable(free, s) || installable(licensed, s))
	}
	return &model.Counts{
		Generated: model.Generated, SchemaVersion: model.CountsVersion, GeneratedFrom: from,
		Free: *fc, Pro: *pc,
		Overlap:    model.Overlap{SharedSlugs: shared, TierPairs: shared, Duplicates: shared},
		Totals:     model.Totals{Entries: fc.Entries + pc.Entries - len(shared), Installable: total},
		Advertised: total,
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// tierCount counts one registry.
func tierCount(r *model.Registry) *model.TierCount {
	tc := &model.TierCount{Entries: len(r.Plugins), NonInstallable: []string{}}
	for s, e := range r.Plugins {
		if e.Installable != nil && !*e.Installable {
			tc.NonInstallable = append(tc.NonInstallable, s)
		}
	}
	sort.Strings(tc.NonInstallable)
	tc.Installable = tc.Entries - len(tc.NonInstallable)
	return tc
}
