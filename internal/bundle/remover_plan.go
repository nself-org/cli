// remover_plan.go — the plan, the data policy and the ledger settlement of
// `nself bundle remove` (P7-PLUG-20).
//
// Purpose: keep Remove short. Holds the pure planning step (what the ledger
// lets this bundle remove), the data-handling rules, the --delete-data
// confirmation gate and the ledger bookkeeping after removals.
// Inputs: a Bundle, a ledger.Ledger and the plugin directory.
// Outputs: a removePlan, the data decision, and the ledger to write.
// Constraints: computePlan and settleLedger do no I/O beyond stat calls on the
// plugin directory; the caller holds the ledger lock for real runs.
package bundle

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
)

// errBundleNotRecorded is the sentinel computePlan returns, and Remove's
// ledger Update propagates, when the ledger has no record of the bundle.
var errBundleNotRecorded = errors.New("bundle is not recorded in the bundle ledger")

// KeptPlugin is a bundle member that stays installed, and why.
type KeptPlugin struct {
	Name   string
	Reason string // e.g. "installed_by: chat", "explicit"
}

// removePlan is what a remove call would do, computed from one ledger snapshot.
type removePlan struct {
	work         ledger.Ledger // the snapshot with the bundle forgotten
	Planned      []string
	Kept         []KeptPlugin
	NotInstalled []string
}

func (r *RemoveResult) adopt(p removePlan) {
	r.Planned, r.Kept, r.NotInstalled = p.Planned, p.Kept, p.NotInstalled
}

// computePlan asks the ledger which plugins the bundle may release (installed_by
// becomes empty and not explicit) and intersects that with what is on disk.
// Planned is in reverse bundle order, then any released plugin the bundle no
// longer lists. A member that stays gets its reason from the original ledger.
func computePlan(b Bundle, l ledger.Ledger, pluginDir string) (removePlan, error) {
	work := l.Clone()
	released, err := work.RemoveBundle(b.Slug)
	if err != nil {
		return removePlan{}, errBundleNotRecorded
	}
	// A released plugin the catalog lists under another recorded bundle stays: the
	// ledger lost that reference (a forced plugin remove, then a reinstall).
	memberOf := map[string]string{}
	released = keepCatalogMembers(&work, l, released, memberOf)
	rel := make(map[string]bool, len(released))
	for _, n := range released {
		rel[n] = true
	}
	plan := removePlan{work: work}
	member := make(map[string]bool, len(b.Plugins))
	for _, name := range b.Plugins {
		key := strings.ToLower(name)
		member[key] = true
		switch {
		case !isAlreadyInstalled(pluginDir, name):
			plan.NotInstalled = append(plan.NotInstalled, name)
		case !rel[key]:
			plan.Kept = append(plan.Kept, KeptPlugin{Name: name, Reason: keptReason(l, b.Slug, key, memberOf)})
		}
	}
	for i := len(b.Plugins) - 1; i >= 0; i-- {
		if name := b.Plugins[i]; rel[strings.ToLower(name)] && isAlreadyInstalled(pluginDir, name) {
			plan.Planned = append(plan.Planned, name)
		}
	}
	for _, n := range released { // sorted; no longer listed by the bundle
		if !member[n] && isAlreadyInstalled(pluginDir, n) {
			plan.Planned = append(plan.Planned, n)
		}
	}
	return plan, nil
}

// keepCatalogMembers takes out of released every plugin that the catalog lists
// as a member of a bundle still recorded in work, and restores its record there
// (facts from the original record, explicit=false, installed_by = those
// bundles). memberOf receives "<bundle>, <bundle>" per kept plugin.
func keepCatalogMembers(work *ledger.Ledger, orig ledger.Ledger, released []string, memberOf map[string]string) []string {
	recorded := make([]string, 0, len(work.Bundles))
	for slug := range work.Bundles {
		recorded = append(recorded, slug)
	}
	sort.Strings(recorded)
	var out []string
	for _, name := range released {
		var holders []string
		for _, slug := range recorded {
			if other, ok := Get(slug); ok && containsFold(other.Plugins, name) {
				holders = append(holders, slug)
			}
		}
		if len(holders) == 0 {
			out = append(out, name)
			continue
		}
		rec := orig.Plugins[name]
		rec.InstalledBy, rec.Explicit = holders, false
		work.Plugins[name] = rec
		memberOf[name] = strings.Join(holders, ", ")
	}
	return out
}

func containsFold(list []string, name string) bool {
	for _, x := range list {
		if strings.EqualFold(x, name) {
			return true
		}
	}
	return false
}

// keptReason says why a plugin stays when bundle slug is removed.
func keptReason(l ledger.Ledger, slug, key string, memberOf map[string]string) string {
	if m, ok := memberOf[key]; ok {
		return "member of " + m
	}
	rec, ok := l.Plugins[key]
	if !ok {
		return "not recorded in the bundle ledger"
	}
	var parts []string
	var others []string
	for _, b := range rec.InstalledBy {
		if b != slug {
			others = append(others, b)
		}
	}
	if len(others) > 0 {
		parts = append(parts, "installed_by: "+strings.Join(others, ", "))
	}
	if rec.Explicit {
		parts = append(parts, "explicit")
	}
	if len(parts) == 0 {
		return "not released by the ledger"
	}
	return strings.Join(parts, "; ")
}

// settleLedger leaves l (the ledger Update will write) reflecting the call.
// Full success: the snapshot with the bundle forgotten. A failure: the bundle
// stays recorded and only the records of plugins actually removed are dropped,
// so a rerun of the same command continues from there.
func settleLedger(l *ledger.Ledger, work ledger.Ledger, removed []string, failed bool) {
	if !failed {
		*l = work
		return
	}
	for _, n := range removed {
		delete(l.Plugins, strings.ToLower(n))
	}
}

// dataPolicy decides whether plugin data is deleted, and whether the v1.4
// default warning is due.
func dataPolicy(b Bundle, opts RemoveOpts) (deleteData, warn bool) {
	if opts.DeleteData {
		return true, false
	}
	// compat.V15(P7-PLUG-20): bundle remove drops plugin data by default (paid bundle) -> keeps it unless --delete-data
	if compat.V15() || b.IsFree() {
		return false, false
	}
	return !opts.KeepData, !opts.KeepData
}

// confirmDeleteData is the gate before anything changes: an explicit
// --delete-data needs --yes or a terminal confirmation, else E403 (exit 4).
// A dry run changes nothing and needs none.
func confirmDeleteData(b Bundle, opts RemoveOpts) error {
	if !opts.DeleteData || opts.DryRun || opts.Yes {
		return nil
	}
	prompt := fmt.Sprintf("Delete the data of the plugins bundle %s releases (schema/tables)? This cannot be undone.", b.Slug)
	if opts.Confirm != nil && opts.Confirm(prompt) {
		return nil
	}
	return errs.New("E403", "bundle remove --delete-data deletes plugin data and was not confirmed").
		WithFix("pass --yes to confirm, or run it on a terminal and answer y")
}

// unrecordedBundle handles a bundle the ledger does not record: v1.5 fails with
// E134, v1.4 warns and removes nothing.
func unrecordedBundle(out io.Writer, b Bundle, ledgerPath string, cause error) error {
	if !errors.Is(cause, errBundleNotRecorded) {
		return cause
	}
	// compat.V15(P7-PLUG-20): bundle remove of an unrecorded bundle removes its members -> fails with E134
	if compat.V15() {
		return errs.Newf("E134", "bundle %s is not recorded as installed in %s", b.Slug, ledgerPath)
	}
	_, _ = fmt.Fprintf(out, "warning: bundle %s is not recorded as installed in %s; nothing removed.\n", b.Slug, ledgerPath)
	return nil
}

// printRemovePlan prints the plan: what goes, what stays and why, and the data rule.
func printRemovePlan(out io.Writer, b Bundle, r *RemoveResult, opts RemoveOpts) {
	header := "Remove plan"
	if opts.DryRun {
		header = "Remove plan (dry-run)"
	}
	_, _ = fmt.Fprintf(out, "%s for bundle %s (%s)\n", header, b.Name, b.Slug)
	switch {
	case !r.KeepData:
		_, _ = fmt.Fprintln(out, "  data: drop plugin schema/tables")
	case opts.KeepData:
		_, _ = fmt.Fprintln(out, "  data: preserve (--keep-data)")
	default:
		_, _ = fmt.Fprintln(out, "  data: preserve (pass --delete-data to drop it)")
	}
	if len(r.Planned) == 0 {
		_, _ = fmt.Fprintln(out, "  (no installed plugins to remove)")
	}
	for _, name := range r.Planned {
		_, _ = fmt.Fprintf(out, "  • %s\n", name)
	}
	kept := append([]KeptPlugin(nil), r.Kept...)
	sort.Slice(kept, func(i, j int) bool { return kept[i].Name < kept[j].Name })
	for _, k := range kept {
		_, _ = fmt.Fprintf(out, "  kept: %s (%s)\n", k.Name, k.Reason)
	}
	if len(r.NotInstalled) > 0 {
		_, _ = fmt.Fprintf(out, "  not-installed: %s\n", strings.Join(r.NotInstalled, ", "))
	}
}
