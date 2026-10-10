// Mutations: the three edits P7-PLUG-19 (app-ready) and P7-PLUG-20 (refcounted
// remove) make to a Ledger.
//
// Purpose: keep the installed_by / explicit rules in one place (PLUG D6): a
// bundle remove drops a plugin only when installed_by becomes empty AND
// explicit is false.
// Inputs: a *Ledger (from Store.Update) and the bundle or plugin to record.
// Outputs: the Ledger is changed in place; RemoveBundle returns the plugins the
// caller may now remove.
// Constraints: pure, no I/O, and nothing here changes install or remove
// behaviour; callers decide what to do with RemoveBundle's answer.
package ledger

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// AddBundle records bundle slug as installed at `at` with the given member
// plugins. Existing plugin records keep their explicit flag; the bundle is
// added to their installed_by. A new plugin record is explicit=false. Members
// may be passed by name only: a fact the input does not carry (version, checksum,
// a tier other than free or licensed) keeps its recorded value. Re-adding a
// bundle that is already recorded keeps its first installed_at.
func (l *Ledger) AddBundle(slug string, members []InstalledPlugin, at time.Time) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	l.normalize()
	if _, ok := l.Bundles[slug]; !ok {
		l.Bundles[slug] = BundleRecord{InstalledAt: at.UTC().Format(time.RFC3339)}
	}
	for _, m := range members {
		name := strings.ToLower(strings.TrimSpace(m.Name))
		if name == "" {
			continue
		}
		rec := l.Plugins[name] // zero value when absent: explicit=false
		rec.InstalledBy = appendUnique(rec.InstalledBy, slug)
		fillFacts(&rec, m)
		l.Plugins[name] = rec
	}
	l.normalize()
}

// RemoveBundle forgets bundle slug and returns, sorted, the plugins that no
// installed bundle references any more and that were not installed explicitly:
// those are the ones a bundle remove may delete. A plugin still referenced by
// another bundle, or explicit, stays in the ledger and is not returned. An
// unknown bundle is an error and changes nothing.
func (l *Ledger) RemoveBundle(slug string) ([]string, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if _, ok := l.Bundles[slug]; !ok {
		return nil, fmt.Errorf("ledger: bundle %q is not recorded as installed", slug)
	}
	l.normalize()
	delete(l.Bundles, slug)
	removable := []string{}
	for name, rec := range l.Plugins {
		kept := make([]string, 0, len(rec.InstalledBy))
		for _, b := range rec.InstalledBy {
			if b != slug {
				kept = append(kept, b)
			}
		}
		rec.InstalledBy = kept
		if len(kept) == 0 && !rec.Explicit {
			delete(l.Plugins, name)
			removable = append(removable, name)
			continue
		}
		l.Plugins[name] = rec
	}
	sort.Strings(removable)
	return removable, nil
}

// MarkExplicit records that the owner installed the plugin on its own, so a
// bundle remove keeps it. A plugin not yet recorded is added with an empty
// installed_by.
func (l *Ledger) MarkExplicit(p InstalledPlugin) {
	l.normalize()
	name := strings.ToLower(strings.TrimSpace(p.Name))
	rec := l.Plugins[name]
	rec.Explicit = true
	fillFacts(&rec, p)
	l.Plugins[name] = rec
	l.normalize()
}

// fillFacts copies the facts p carries into rec and leaves the rest alone. A
// new record (Tier empty) defaults to free. Tier changes only to free or
// licensed; version and checksum change only to a non-empty value.
func fillFacts(rec *PluginRecord, p InstalledPlugin) {
	switch p.Tier {
	case TierFree, TierLicensed:
		rec.Tier = p.Tier
	default:
		if rec.Tier == "" {
			rec.Tier = TierFree
		}
	}
	if p.Version != "" {
		rec.Version = p.Version
	}
	if cs := strings.ToLower(strings.TrimPrefix(p.Checksum, "sha256:")); cs != "" && checksumRE.MatchString(cs) {
		rec.Checksum = cs
	}
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
