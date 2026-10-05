// Schema registration: portable bundle manifest (contract:cli.portable-export v1,
// P7-ADOPT-23).
//
// Purpose: schemas/portable-export.v1.schema.json is generated from
// portable.Manifest. Patterns, enums and the version const come from exported
// values of internal/portable; arrays are never null in a written manifest
// (Manifest.Sort replaces nil slices), so the schema says `array`.
package main

import (
	"github.com/nself-org/cli/internal/portable"
)

func init() {
	arr := map[string]any{"type": "array"}
	ovs := []Override{
		{Pointer: "/properties/_format", Set: map[string]any{"const": portable.Format}},
		{Pointer: "/properties/schema_version", Set: map[string]any{"pattern": portable.VersionPattern}},
		{Pointer: "/properties/created_at", Set: map[string]any{"format": "date-time"}},
		{Pointer: "/properties/producer/properties/source", Set: map[string]any{"enum": strs(portable.Sources...)}},
		{Pointer: "/properties/exemptions/items/properties/kind", Set: map[string]any{"enum": strs(portable.ExemptionKinds...)}},
		{Pointer: "/properties/source_counts/items/properties/kind", Set: map[string]any{"enum": strs(portable.SourceCountKinds...)}},
		{Pointer: "/properties/db/properties/tables/items/properties/hash", Set: map[string]any{"pattern": portable.HashPattern}},
		{Pointer: "/properties/storage/properties/objects/items/properties/member", Set: map[string]any{"pattern": portable.StorageMemberPattern}},
		{Pointer: "/properties/storage/properties/objects/items/properties/sha256", Set: map[string]any{"pattern": portable.SHA256Pattern}},
		{Pointer: "/properties/files/items/properties/sha256", Set: map[string]any{"pattern": portable.SHA256Pattern}},
		{Pointer: "/properties/auth/properties/hash_algorithms", Set: map[string]any{"type": "object"}},
	}
	// Every []T field of Manifest.
	for _, p := range []string{
		"/properties/db/properties/schemas",
		"/properties/db/properties/tables",
		"/properties/db/properties/tables/items/properties/pk",
		"/properties/auth/properties/reset_required",
		"/properties/storage/properties/buckets",
		"/properties/storage/properties/objects",
		"/properties/exemptions",
		"/properties/source_counts",
		"/properties/compat",
		"/properties/files",
	} {
		ovs = append(ovs, Override{Pointer: p, Set: arr})
	}
	Register(Spec{Out: "portable-export.v1.schema.json", Type: portable.Manifest{}, Overrides: ovs})
}
