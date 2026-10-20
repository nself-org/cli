package commands

import "github.com/nself-org/cli/internal/controlplane"

// deployPreflightInventory scopes legacy deploy and ops architecture checks to
// the selected host. Architecture uses the inventory key when present, then
// NSELF_DEPLOY_SSH_KEY, then the same default key as the deploy transport.
func deployPreflightInventory(inv *controlplane.Inventory, target, host string) *controlplane.Inventory {
	server := controlplane.Server{Name: target + "-app", Role: controlplane.RoleApp, Host: host, Primary: true}
	if inv != nil {
		for _, candidate := range inv.Environments[target].Servers {
			if candidate.Host == host {
				server = candidate
				break
			}
		}
	}
	return &controlplane.Inventory{Environments: map[string]controlplane.Environment{target: {Name: target, Kind: "remote", Servers: []controlplane.Server{server}}}}
}
