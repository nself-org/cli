package main

import "github.com/nself-org/cli/cmd/commands"

func init() {
	typeOf := commands.JSONDataTypes()["deploy targets"]
	Register(Spec{Out: "deploy-targets.v1.schema.json", Type: typeOf, Overrides: []Override{
		{Pointer: "/properties/targets", Set: map[string]any{"type": "array"}},
		{Pointer: "/properties/warnings", Set: map[string]any{"type": "array"}},
		{Pointer: "/properties/targets/items/properties/addresses", Set: map[string]any{"type": "array"}},
		{Pointer: "/properties/targets/items/properties/host_key_fingerprints", Set: map[string]any{"type": "array"}},
	}})
}
