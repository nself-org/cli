// Overrides for the data schemas of the read-only pilot commands (P7-REG-09).
//
// Purpose: inference from the Go types gives `type: string` for the closed
// string sets and `["null","array"]` for slices. The pilots never emit null
// arrays and only emit the listed values, so the schemas say so.
//
// Inputs: commands.PilotJSONEnums (the same constants the code uses).
// Outputs: type overrides applied to every schema generated from the pilot
// data types (see commands.go, which registers them).
// Constraints: the Go types and their JSON shape are never changed to get
// here; only the generated schema narrows.
package main

import "github.com/nself-org/cli/cmd/commands"

func init() {
	enums := commands.PilotJSONEnums()
	types := commands.JSONDataTypes()
	arr := map[string]any{"type": "array"}
	enum := func(vals []string) map[string]any { return map[string]any{"enum": vals} }

	RegisterTypeOverrides(types["status"],
		Override{Pointer: "/properties/state", Set: enum(enums.StatusState)},
		Override{Pointer: "/properties/services", Set: arr},
	)
	RegisterTypeOverrides(types["doctor"],
		Override{Pointer: "/properties/state", Set: enum(enums.DoctorState)},
		Override{Pointer: "/properties/checks", Set: arr},
	)
	RegisterTypeOverrides(types["config list"],
		Override{Pointer: "/properties/keys", Set: arr},
		Override{Pointer: "/properties/keys/items/properties/source", Set: enum(enums.ConfigListSource)},
	)
}
