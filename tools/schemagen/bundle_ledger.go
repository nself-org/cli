// Schema registration: bundle install-state ledger (contract:cli.bundle-ledger v1,
// P7-PLUG-18, PLUG D6).
//
// Purpose: schemas/bundle-ledger.v1.schema.json is generated from
// ledger.Ledger. Patterns, enums and the constants come from exported values of
// internal/bundle/ledger, never from duplicated literals.
// Note: a JSON Schema cannot say "_generated is the first key"; the Go reader
// (ledger.Parse) and its tests enforce that.
package main

import (
	"github.com/nself-org/cli/internal/bundle/ledger"
)

func init() {
	slugKeys := map[string]any{"pattern": ledger.SlugPattern}
	Register(Spec{
		Out:  "bundle-ledger.v1.schema.json",
		Type: ledger.Ledger{},
		Overrides: []Override{
			{Pointer: "/properties/_generated", Set: map[string]any{"const": ledger.Generated}},
			{Pointer: "/properties/schema_version", Set: map[string]any{"const": ledger.SchemaVersion}},
			{Pointer: "/properties/bundles", Set: map[string]any{"propertyNames": slugKeys}},
			{Pointer: "/properties/bundles/additionalProperties/properties/installed_at", Set: map[string]any{"format": "date-time"}},
			{Pointer: "/properties/plugins", Set: map[string]any{"propertyNames": slugKeys}},
			{Pointer: "/properties/plugins/additionalProperties/properties/installed_by", Set: map[string]any{
				"type": "array", "uniqueItems": true, "items": map[string]any{"pattern": ledger.SlugPattern, "type": "string"}}},
			{Pointer: "/properties/plugins/additionalProperties/properties/tier", Set: map[string]any{"enum": strs(ledger.Tiers...)}},
			{Pointer: "/properties/plugins/additionalProperties/properties/checksum", Set: map[string]any{"pattern": ledger.ChecksumPattern}},
		},
	})
}
