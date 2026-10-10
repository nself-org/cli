// Schema registration: project lock nself.lock.yaml (contract:cli.lockfile v1,
// P7-LIVE-09, EPIC D10).
//
// Purpose: schemas/lockfile.v1.schema.json is generated from lock.Lock. Patterns
// and enums come from exported values of internal/lock, never from duplicated
// literals. The file is YAML; the schema describes its parsed document. Its
// first-line header comment is not part of the document.
package main

import (
	"github.com/nself-org/cli/internal/lock"
)

func init() {
	keys := map[string]any{"pattern": lock.KeyPattern}
	Register(Spec{
		Out:  "lockfile.v1.schema.json",
		Type: lock.Lock{},
		Overrides: []Override{
			{Pointer: "/properties/schema_version", Set: map[string]any{"const": lock.SchemaVersion}},
			{Pointer: "/properties/cli", Set: map[string]any{"pattern": lock.CLIPattern}},
			{Pointer: "/properties/images", Set: map[string]any{"propertyNames": keys, "additionalProperties": map[string]any{"type": "string", "pattern": lock.PinnedPattern}}},
			{Pointer: "/properties/unpinned", Set: map[string]any{"propertyNames": keys, "additionalProperties": map[string]any{"type": "string", "pattern": lock.UnpinnedPattern}}},
			{Pointer: "/properties/plugins", Set: map[string]any{"propertyNames": keys}},
			{Pointer: "/properties/plugins/additionalProperties/properties/sha256", Set: map[string]any{"pattern": lock.ChecksumPattern}},
			{Pointer: "/properties/plugins/additionalProperties/properties/tier", Set: map[string]any{"enum": strs(lock.Tiers...)}},
			{Pointer: "/properties/bundles", Set: map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string", "pattern": lock.KeyPattern}}},
		},
	})
}
