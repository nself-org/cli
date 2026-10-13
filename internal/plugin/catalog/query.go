package catalog

// query.go — lookups over a decoded catalog.
//
// Purpose: the three questions the CLI asks: which entry is slug X, which
// plugin provides command C, and which entries match a search word.
// Inputs: a *Catalog. Outputs: Entry values in catalog order (sorted by slug,
// then tier, as the generator writes them).

import "strings"

// Lookup returns the entry for slug, preferring the free tier of a tier pair.
func (c *Catalog) Lookup(slug string) (Entry, bool) {
	var found Entry
	ok := false
	for _, p := range c.Plugins {
		if p.Slug != slug {
			continue
		}
		if !ok || p.Tier == TierFree {
			found, ok = p, true
		}
	}
	return found, ok
}

// FindByCommand returns the installable plugin whose commands.command is cmd,
// preferring the free tier when both tiers provide it.
func (c *Catalog) FindByCommand(cmd string) (Entry, bool) {
	var found Entry
	ok := false
	for _, p := range c.Plugins {
		if cmd == "" || !p.Installable || p.Commands == nil || p.Commands.Command != cmd {
			continue
		}
		if !ok || (p.Tier == TierFree && found.Tier != TierFree) {
			found, ok = p, true
		}
	}
	return found, ok
}

// Search returns the entries whose slug, description, category or bundle
// contains q, case-insensitively. An empty q matches everything. tier narrows
// to TierFree or TierLicensed; empty means both.
func (c *Catalog) Search(q, tier string) []Entry {
	q = strings.ToLower(strings.TrimSpace(q))
	var out []Entry
	for _, p := range c.Plugins {
		if tier != "" && p.Tier != tier {
			continue
		}
		if q == "" || p.matches(q) {
			out = append(out, p)
		}
	}
	return out
}

func (p Entry) matches(q string) bool {
	for _, f := range append([]string{p.Slug, p.Description, p.Category}, p.Bundles...) {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}
