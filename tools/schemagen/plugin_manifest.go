// Generated schema for plugin.json manifest v2 (contract plugin.manifest, P7-PLUG-01).
//
// Purpose: schemas/plugin-manifest.v2.schema.json comes from the Go types in
// internal/plugin/manifestv2; enums and patterns come from that package's
// exported values, never from copies. Nullable pointers stay nullable only where
// the contract says object|null (commands, schema, migrations, seed).
package main

import "github.com/nself-org/cli/internal/plugin/manifestv2"

func anyList(v []string) []any {
	out := make([]any, len(v))
	for i, s := range v {
		out[i] = s
	}
	return out
}

func set(ptr string, kv map[string]any) Override { return Override{Pointer: ptr, Set: kv} }

func init() {
	const sub = "/properties/commands/properties/subcommands/items"
	ovs := []Override{
		set("", map[string]any{"description": "nSelf plugin manifest v2 (plugin.json with manifest_version 2). Generated from internal/plugin/manifestv2."}),
		set("/properties/manifest_version", map[string]any{"const": manifestv2.ManifestVersion, "description": "Always 2."}),
		set("/properties/name", map[string]any{"pattern": manifestv2.NamePattern, "description": "Plugin slug, unique per tier."}),
		set("/properties/version", map[string]any{"pattern": manifestv2.VersionPattern, "description": "Plugin version (semver)."}),
		set("/properties/license", map[string]any{"enum": anyList(manifestv2.Licenses), "description": "free or licensed. Bundle membership is not a manifest field (ADR 0008)."}),
		set("/properties/license_spdx", map[string]any{"description": "Licence text carried from a v1 file (an SPDX identifier or Source-Available); shown by plugin info."}),
		set("/properties/maturity", map[string]any{"enum": anyList(manifestv2.Maturities), "description": "Lifecycle. deferred requires deprecation.state."}),
		set("/properties/installable", map[string]any{"description": "Defaults to true when absent."}),
		set("/properties/service", map[string]any{"type": "object", "description": "How the plugin runs. kind compose requires compose and healthcheck."}),
		set("/properties/service/properties/kind", map[string]any{"enum": anyList(manifestv2.ServiceKinds)}),
		set("/properties/commands", map[string]any{"description": "CLI surface mounted as `nself <command>`; null when the plugin adds no command."}),
		set("/properties/commands/properties/command", map[string]any{"pattern": manifestv2.NamePattern, "description": "Must not be a core command verb (E113)."}),
		set("/properties/commands/properties/binary", map[string]any{"pattern": manifestv2.BinaryPattern}),
		set("/properties/commands/properties/confirm/properties/flags", map[string]any{"type": "array"}),
		set(sub+"/properties/name", map[string]any{"pattern": `^[a-z][a-z0-9-]*( [a-z][a-z0-9-]*)*$`, "description": "One or more space-separated segments."}),
		set(sub+"/properties/confirm/properties/flags", map[string]any{"type": "array"}),
		set("/properties/schema", map[string]any{"description": "np_<name with - replaced by _>."}),
		set("/properties/migrations/properties/dir", map[string]any{"const": "migrations"}),
		set("/properties/migrations/properties/apply", map[string]any{"const": "boot"}),
		set("/properties/docs_url", map[string]any{"pattern": `^https://`}),
		set("/properties/deprecation/properties/state", map[string]any{"enum": anyList(manifestv2.States)}),
		set("/properties", map[string]any{"permissions": map[string]any{
			"oneOf": []any{
				map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
			},
			"description": "Same key and shape as v1: canonical permission strings, or categories mapped to actions.",
		}}),
	}
	for _, p := range []string{"/properties/commands", sub} {
		ovs = append(ovs,
			set(p+"/properties/side_effect", map[string]any{"enum": anyList(manifestv2.SideEffects)}),
			set(p+"/properties/output", map[string]any{"enum": anyList(manifestv2.Outputs)}),
			set(p+"/properties/json", map[string]any{"enum": anyList(manifestv2.JSONModes)}),
			set(p+"/properties/surface", map[string]any{"enum": anyList(manifestv2.Surfaces), "default": manifestv2.SurfaceAll}),
		)
	}
	for _, p := range []string{"flags", "args"} {
		ovs = append(ovs, set(sub+"/properties/"+p, map[string]any{"type": "array"}))
	}
	ovs = append(ovs,
		set(sub+"/properties/flags/items/properties/side_effect", map[string]any{"enum": anyList(manifestv2.SideEffects)}),
		set(sub+"/properties/flags/items/properties/output", map[string]any{"enum": anyList(manifestv2.Outputs)}),
		set(sub+"/properties/flags/items/properties/json", map[string]any{"enum": anyList(manifestv2.JSONModes)}),
	)
	Register(Spec{Out: "plugin-manifest.v2.schema.json", Type: manifestv2.Manifest{}, Overrides: ovs})
}
