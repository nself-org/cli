package main

// Purpose: schemas/invoke/confirmation.v1.schema.json, the data of a machine
// confirmation (contract:cli.machine-request v1, P7-SURF-24).
// Inputs: invoke.Confirmation. Outputs: one generated schema.
// Constraints: the plan member is a change-plan v1 object or null; the shape of
// that object belongs to P7-LIVE-01, so it is typed as object-or-null here.

import "github.com/nself-org/cli/internal/invoke"

func init() {
	Register(Spec{Out: "invoke/confirmation.v1.schema.json", Type: invoke.Confirmation{}, Overrides: []Override{
		{Pointer: "", Set: map[string]any{"additionalProperties": false}},
		{Pointer: "/properties/confirmation_required", Set: map[string]any{"const": true}},
		{Pointer: "/properties/kind", Set: map[string]any{"enum": strs("plan", "request")}},
		{Pointer: "/properties/confirm", Set: map[string]any{"pattern": "^[0-9a-f]{64}$"}},
		{Pointer: "/properties/side_effect", Set: map[string]any{"enum": strs("write", "remote", "destructive")}},
		// plan is an `any` field, which infers as a bare true: replace it whole.
		{Pointer: "/properties", Set: map[string]any{"plan": map[string]any{"type": []any{"object", "null"}}}},
	}})
}
