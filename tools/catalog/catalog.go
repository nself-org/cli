package main

// catalog.go: join the free registry, the licensed registry and bundles.json
// into catalog.json (contract:plugins.catalog v1).
//
// Purpose: one catalog for the worker, the CLI client and the web loader. The
// public repo never parses licensed manifests, so both sides are read from
// registry.json files this tool wrote (each entry's `catalog` block carries the
// manifest-only columns).
// Inputs: two model.Registry values, model.Bundles, the run's GeneratedFrom.
// Outputs: model.Catalog, with counts embedded.
// Constraints: rows sorted by (slug, tier); a tier-pair slug has one row per
// tier and counts once; every problem is reported, sorted.

import (
	"fmt"
	"sort"

	"github.com/nself-org/cli/tools/catalog/model"
)

// buildCatalog joins the inputs.
func buildCatalog(free, licensed *model.Registry, b *model.Bundles, from model.GeneratedFrom) (*model.Catalog, []string) {
	c := &model.Catalog{Generated: model.Generated, SchemaVersion: model.CatalogVersion,
		GeneratedFrom: from, Plugins: []model.CatalogPlugin{}, Bundles: map[string]model.CatalogBundle{}}
	var probs []string
	shared := sharedSlugs(free, licensed)
	for _, side := range []struct {
		reg  *model.Registry
		tier string
	}{{free, model.TierFree}, {licensed, model.TierLicensed}} {
		for _, slug := range sortedKeys(side.reg.Plugins) {
			row, p := rowOf(slug, side.tier, side.reg.Plugins[slug], b)
			if p != "" {
				probs = append(probs, p)
				continue
			}
			row.TierPair = shared[slug]
			c.Plugins = append(c.Plugins, row)
		}
	}
	sort.SliceStable(c.Plugins, func(i, j int) bool {
		if c.Plugins[i].Slug != c.Plugins[j].Slug {
			return c.Plugins[i].Slug < c.Plugins[j].Slug
		}
		return c.Plugins[i].Tier < c.Plugins[j].Tier
	})
	probs = append(probs, checkPairs(free, licensed)...)
	for _, k := range sortedKeys(b.Bundles) {
		be := b.Bundles[k]
		tier, _ := catalogTier(be.Tier) // validated by loadBundles
		reg := free
		if tier == model.TierLicensed {
			reg = licensed
		}
		members := append([]string{}, be.Plugins...)
		sort.Strings(members)
		for _, p := range members {
			if _, ok := reg.Plugins[p]; !ok {
				probs = append(probs, fmt.Sprintf("bundle %s lists %s, which is not in the %s registry", k, p, tier))
			}
		}
		c.Bundles[k] = model.CatalogBundle{Display: be.Display, Tier: tier, Plugins: members}
	}
	c.Counts = *buildCounts(free, licensed, from)
	return c, probs
}

// rowOf maps one registry entry to a catalog row.
func rowOf(slug, tier string, e model.RegistryEntry, b *model.Bundles) (model.CatalogPlugin, string) {
	if e.Catalog == nil {
		return model.CatalogPlugin{}, fmt.Sprintf("%s (%s): registry entry has no catalog block; regenerate the registry with this tool", slug, tier)
	}
	in := e.Catalog
	row := model.CatalogPlugin{
		Slug: slug, Tier: tier, Version: e.Version, Description: e.Description, Category: e.Category,
		Maturity: in.Maturity, Installable: e.Installable == nil || *e.Installable,
		ServiceKind: in.ServiceKind, Commands: in.Commands, Requires: in.Requires,
		Bundles: membership(b, slug, tier), Checksum: e.Checksum, ReleaseSignature: e.ReleaseSignature,
		TarballURL: firstNonEmpty(e.TarballURL, e.DownloadURL),
	}
	if row.Bundles == nil {
		row.Bundles = []string{}
	}
	if in.Schema != "" {
		row.Schema = &in.Schema
	}
	if in.DocsURL != "" {
		row.DocsURL = &in.DocsURL
	}
	return row, ""
}

// sharedSlugs is the set of slugs present in both registries.
func sharedSlugs(a, b *model.Registry) map[string]bool {
	out := map[string]bool{}
	for s := range a.Plugins {
		if _, ok := b.Plugins[s]; ok {
			out[s] = true
		}
	}
	return out
}

// checkPairs requires tier_pair on both sides of every shared slug (one
// two-tier product, counted once) and on no unshared slug.
func checkPairs(free, licensed *model.Registry) []string {
	var probs []string
	shared := sharedSlugs(free, licensed)
	for _, slug := range sortedKeys(shared) {
		if !free.Plugins[slug].TierPair || !licensed.Plugins[slug].TierPair {
			probs = append(probs, slug+": present in both registries without tier_pair on both sides")
		}
	}
	for _, side := range []*model.Registry{free, licensed} {
		for _, slug := range sortedKeys(side.Plugins) {
			if side.Plugins[slug].TierPair && !shared[slug] {
				probs = append(probs, slug+": tier_pair is set but the slug is in one registry only")
			}
		}
	}
	return probs
}
