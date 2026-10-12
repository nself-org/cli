// remover_deps.go — keep bundle members that a kept plugin depends on (D-0325).
//
// Purpose: `bundle remove` releases a member when no bundle and no explicit
// install needs it, but a plugin outside the plan can still list that member
// in its plugin.json dependencies. Removing it would break that plugin.
// Inputs: a removePlan, the original ledger, the plugin directory and a
// reverse-dependency scan (plugin.ReverseDependencies unless a test swaps it).
// Outputs: the plan with such members moved from Planned to Kept (reason
// "needed by <plugins>; recorded explicit") and their records in plan.work
// restored as explicit, so a later bundle remove keeps them too.
// Constraints: iterates to a fixpoint (a kept plugin is itself an outside
// dependent of what it needs). A scan error refuses the plan: nothing is
// removed. No I/O beyond the scan.
package bundle

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/plugin"
)

// reverseDepsFunc lists the installed plugins that depend on name.
type reverseDepsFunc func(pluginDir, name string) ([]string, error)

// keepDependedOn moves every planned member that a plugin outside the plan
// depends on from Planned to Kept, until nothing changes.
func keepDependedOn(plan *removePlan, orig ledger.Ledger, pluginDir string, scan reverseDepsFunc) error {
	if scan == nil {
		scan = plugin.ReverseDependencies
	}
	planned := make(map[string]bool, len(plan.Planned))
	for _, n := range plan.Planned {
		planned[strings.ToLower(n)] = true
	}
	for changed := true; changed; {
		changed = false
		for _, name := range plan.Planned {
			key := strings.ToLower(name)
			if !planned[key] {
				continue
			}
			deps, err := scan(pluginDir, name)
			if err != nil {
				return fmt.Errorf("checking which installed plugins depend on %s: %w", name, err)
			}
			outside := outsidePlan(deps, planned)
			if len(outside) == 0 {
				continue
			}
			delete(planned, key)
			changed = true
			plan.Kept = append(plan.Kept, KeptPlugin{Name: name, Reason: "needed by " + strings.Join(outside, ", ") + "; recorded explicit"})
			rec := orig.Plugins[key]
			if rec.Tier == "" {
				rec.Tier = ledger.TierFree
			}
			rec.InstalledBy, rec.Explicit = []string{}, true
			plan.work.Plugins[key] = rec
		}
	}
	left := plan.Planned[:0:0]
	for _, name := range plan.Planned {
		if planned[strings.ToLower(name)] {
			left = append(left, name)
		}
	}
	plan.Planned = left
	return nil
}

// outsidePlan returns the sorted, de-duplicated names in deps that are not
// part of the plan.
func outsidePlan(deps []string, planned map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range deps {
		k := strings.ToLower(d)
		if planned[k] || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}
