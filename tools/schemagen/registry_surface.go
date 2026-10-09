package main

// Purpose: constrain appended registry surface fields in every registry schema.
// Inputs: cmdregistry.Registry type registration. Outputs: enum and shape rules.
// Constraints: share the type override hook with command-registry and help data.

import "github.com/nself-org/cli/internal/cmdregistry"

func init() {
	cmd := "/properties/commands/items/properties/"
	RegisterTypeOverrides(cmdregistry.Registry{},
		Override{Pointer: cmd + "surface", Set: map[string]any{"enum": strs("all", "cli-only")}},
		Override{Pointer: cmd + "confirm", Set: map[string]any{"type": []any{"object", "null"}}},
		Override{Pointer: cmd + "confirm/properties/flags", Set: map[string]any{"type": "array"}},
		Override{Pointer: cmd + "confirm/properties/plan", Set: map[string]any{"type": []any{"object", "null"}}},
	)
}
