package controlplane

import (
	"fmt"
	"sort"

	"github.com/nself-org/cli/internal/errs"
)

// Selector filters inventory targets. Empty fields match all entries.
type Selector struct {
	Env    string
	Tier   Tier
	Server string
}

// Target couples a server with its environment for callers of ResolveTargets.
type Target struct {
	Env    string
	Tier   Tier
	Server Server
}

var roleOrder = map[ServerRole]int{RoleObservability: 0, RoleApp: 1, RoleLB: 2, RoleDB: 3, RoleWorker: 4}

// ResolveTargets returns matching servers sorted by environment, role, name.
func ResolveTargets(inv *Inventory, sel Selector) ([]Target, error) {
	if inv == nil {
		return nil, errs.New("E486", "inventory is nil")
	}
	var out []Target
	for name, env := range inv.Environments {
		if sel.Env != "" && name != sel.Env {
			continue
		}
		tier := normalizedTier(env)
		if sel.Tier != "" && tier != sel.Tier {
			continue
		}
		for _, s := range env.Servers {
			if sel.Server != "" && s.Name != sel.Server {
				continue
			}
			out = append(out, Target{Env: name, Tier: tier, Server: s})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Env != b.Env {
			return a.Env < b.Env
		}
		if roleOrder[a.Server.Role] != roleOrder[b.Server.Role] {
			return roleOrder[a.Server.Role] < roleOrder[b.Server.Role]
		}
		return a.Server.Name < b.Server.Name
	})
	if len(out) == 0 {
		return nil, errs.New("E486", fmt.Sprintf("no target matched env=%q tier=%q server=%q", sel.Env, sel.Tier, sel.Server))
	}
	return out, nil
}
