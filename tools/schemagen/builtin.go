// Built-in schema registrations: envelope, error object, command registry.
//
// Purpose: the three schemas every other contract builds on. Enums, patterns
// and consts that Go types cannot express are Overrides whose values come from
// the exported constants of internal/output, internal/errs, internal/canon and
// internal/cmdregistry; no literal is duplicated here.
package main

import (
	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
)

// errorRef is the $id of the error object schema.
var errorRef = schemaID("error.v1.schema.json")

// strs widens a string list to []any (enum values).
func strs(vals ...string) []any {
	out := make([]any, len(vals))
	for i, v := range vals {
		out[i] = v
	}
	return out
}

// nullable is strs plus null, for fields typed *string.
func nullable(vals ...string) []any { return append(strs(vals...), nil) }

// errorClasses lists every class errs.ClassFor can return, in exit order.
func errorClasses() []any {
	return strs(
		errs.ClassFor(errs.ExitUserError),
		errs.ClassFor(errs.ExitInfraError),
		errs.ClassFor(errs.ExitAuthError),
		errs.ClassFor(errs.ExitDestructiveBlocked),
		errs.ClassFor(errs.ExitDestructiveBlocked+1),
	)
}

// sideEffects, outputs and jsons list the canon enums once.
func sideEffects() []string {
	return []string{canon.SideEffectRead, canon.SideEffectWrite, canon.SideEffectRemote, canon.SideEffectDestructive}
}

func init() {
	Register(Spec{
		Out:  "envelope.v1.schema.json",
		Type: output.Envelope{},
		Overrides: []Override{
			// Exactly one of data/error; the error variant has no data key.
			{Pointer: "", Set: map[string]any{
				"required": []any{"schema_version", "command"},
				"oneOf": []any{
					map[string]any{"required": []any{"data"}},
					map[string]any{"required": []any{"error"}},
				},
			}},
			{Pointer: "/properties", Set: map[string]any{
				"error": map[string]any{"$ref": errorRef},
			}},
			{Pointer: "/properties/schema_version", Set: map[string]any{"const": output.SchemaVersion}},
			{Pointer: "/properties/meta", Set: map[string]any{"type": "object"}},
			{Pointer: "/properties/meta/properties/deprecations", Set: map[string]any{"type": "array"}},
			{Pointer: "/properties/meta/properties/warnings", Set: map[string]any{"type": "array"}},
		},
	})

	Register(Spec{
		Out:  "error.v1.schema.json",
		Type: errs.Detail{},
		Overrides: []Override{
			{Pointer: "/properties/code", Set: map[string]any{"pattern": "^E[0-9]{3}$"}},
			{Pointer: "/properties/class", Set: map[string]any{"enum": errorClasses()}},
			{Pointer: "/properties/exit_code", Set: map[string]any{"minimum": 1, "maximum": 255}},
		},
	})

	cmdItem := "/properties/commands/items/properties/"
	ovs := []Override{
		{Pointer: "/properties/schema_version", Set: map[string]any{"const": cmdregistry.SchemaVersion}},
		{Pointer: "/properties/verbs", Set: map[string]any{"maxItems": canon.MaxVerbs}},
		{Pointer: cmdItem + "canon", Set: map[string]any{"enum": strs(
			canon.CanonCore, canon.CanonSubcommand, canon.CanonPlugin,
			canon.CanonShim, canon.CanonPending, canon.CanonBuiltin)}},
		{Pointer: cmdItem + "side_effect", Set: map[string]any{"enum": strs(sideEffects()...)}},
		{Pointer: cmdItem + "output", Set: map[string]any{"enum": strs(
			canon.OutputDocument, canon.OutputStream, canon.OutputInteractive)}},
		{Pointer: cmdItem + "json", Set: map[string]any{"enum": strs(
			canon.JSONEnvelope, canon.JSONLegacy, canon.JSONNone)}},
		{Pointer: cmdItem + "data_schema", Set: map[string]any{
			"pattern": `^schemas/commands/[a-z0-9-]+\.v[0-9]+\.schema\.json$`}},
		{Pointer: cmdItem + "exit_codes", Set: map[string]any{
			"propertyNames": map[string]any{"pattern": "^[0-9]{1,3}$"}}},
	}
	// Flag appears under root.flags and under every command's flags.
	for _, p := range []string{"/properties/root/properties/flags/items", cmdItem + "flags/items"} {
		ovs = append(ovs,
			Override{Pointer: p + "/properties/side_effect", Set: map[string]any{"enum": nullable(sideEffects()...)}},
			Override{Pointer: p + "/properties/json", Set: map[string]any{"enum": nullable(canon.JSONLegacy, canon.JSONNone)}},
			Override{Pointer: p + "/properties/output", Set: map[string]any{"enum": nullable(canon.OutputStream)}},
		)
	}
	// The committed .github/command-registry.json carries a first _generated
	// key that `help --json` data does not; only this schema admits it.
	ovs = append(ovs, Override{Pointer: "/properties", Set: map[string]any{
		"_generated": map[string]any{"type": "string"}}})
	Register(Spec{Out: "command-registry.v1.schema.json", Type: cmdregistry.Registry{}, Overrides: ovs})
}
