package commands

import "github.com/nself-org/cli/internal/controlplane"

// deployPreflightInventory scopes the architecture check to the deployed host.
// Inputs: loaded project inventory, target name, and normalized SSH host.
// Outputs: one remote environment, preserving matching server metadata.
// Constraints: fallback servers use the app role; the input is unchanged.
func deployPreflightInventory(loaded *controlplane.Inventory, target, host string) *controlplane.Inventory {
	server := controlplane.Server{Name: target + "-app", Role: controlplane.RoleApp, Host: host, Primary: true}
	for _, environment := range loaded.Environments {
		for _, candidate := range environment.Servers {
			if candidate.Host == host {
				server = candidate
			}
		}
	}
	return &controlplane.Inventory{Environments: map[string]controlplane.Environment{
		target: {Name: target, Kind: "remote", Servers: []controlplane.Server{server}},
	}}
}
