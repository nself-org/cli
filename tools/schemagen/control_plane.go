package main

import "github.com/nself-org/cli/internal/controlplane"

func init() {
	Register(Spec{Out: "control-plane.v2.schema.json", Type: controlplane.Inventory{}, Overrides: []Override{
		{Pointer: "/properties/schema_version", Set: map[string]any{"const": 2}},
		{Pointer: "/properties/environments/additionalProperties/properties/tier", Set: map[string]any{"enum": []string{string(controlplane.TierLocal), string(controlplane.TierLocalServers), string(controlplane.TierProd)}}},
		{Pointer: "/properties/environments/additionalProperties/properties/servers/items/properties/arch", Set: map[string]any{"enum": []string{"amd64", "arm64"}}},
	}})
}
