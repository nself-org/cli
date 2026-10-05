// Generated schema for nself.yaml (contract config.nself-yaml, P7-SURF-07).
//
// Purpose: schemas/nself-yaml.v1.schema.json describes exactly what the CLI
// reads from nself.yaml, derived from nsbuild.ProjectManifest by reflection (the
// yaml tags become the property names), plus the extension rule: keys matching
// ^x- are allowed and ignored, at the top level and inside the plugins map.
// A field added to ProjectManifest or ManifestPlugins changes the output, so
// `schemagen -check` fails until the schema is regenerated.
//
// Constraints: ProjectManifest carries yaml tags only, and its plugins field has
// a custom decoder, so the plugins property is replaced whole (list, or map with the ManifestPlugins
// keys) is an override. Null is accepted for every key, as a YAML decoder
// treats an empty value as the zero value.
package main

import (
	"reflect"
	"sort"

	nsbuild "github.com/nself-org/cli/internal/build"
)

// nselfYAMLType returns a struct type with the fields of nsbuild.ProjectManifest
// retagged for JSON, so the inference library sees the YAML key names.
func nselfYAMLType() reflect.Type {
	fs := nsbuild.ManifestFields(reflect.TypeOf(nsbuild.ProjectManifest{}))
	names := make([]string, 0, len(fs))
	for n := range fs {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []reflect.StructField
	for i, n := range names {
		t := fs[n]
		if t == reflect.TypeOf(nsbuild.ManifestPlugins{}) {
			t = reflect.TypeOf((*any)(nil)).Elem() // shape is an override
		}
		out = append(out, reflect.StructField{
			Name: "F" + string(rune('A'+i)), Type: t,
			Tag: reflect.StructTag(`json:"` + n + `,omitempty"`),
		})
	}
	return reflect.StructOf(out)
}

func orNull(t string) any { return []any{t, "null"} }

func init() {
	ext := map[string]any{"^" + nsbuild.ExtensionPrefix: map[string]any{"description": "Extension key: app metadata that nself does not read."}}
	str := map[string]any{"type": "string"}
	strList := map[string]any{"type": "array", "items": str}
	ovs := []Override{
		set("", map[string]any{
			"description":          "nself.yaml: the project manifest nself build reads. Unknown keys are findings; keys starting with x- are extensions and are ignored.",
			"additionalProperties": false, "patternProperties": ext,
		}),
	}
	for n, t := range nsbuild.ManifestFields(reflect.TypeOf(nsbuild.ProjectManifest{})) {
		p := "/properties/" + n
		switch {
		case t == reflect.TypeOf(nsbuild.ManifestPlugins{}):
			sub := map[string]any{}
			for k := range nsbuild.ManifestFields(t) {
				sub[k] = map[string]any{"type": orNull("array"), "items": str}
			}
			ovs = append(ovs, set("/properties", map[string]any{n: map[string]any{
				"description": "Plugins to wire in: a flat list of names, or a map with free and pro lists.",
				"anyOf": []any{
					map[string]any{"type": "null"},
					strList,
					map[string]any{"type": "object", "properties": sub, "additionalProperties": false, "patternProperties": ext},
				},
			}}))
		case t.Kind() == reflect.Slice:
			ovs = append(ovs, set(p, map[string]any{"type": orNull("array"), "items": str}))
		default:
			ovs = append(ovs, set(p, map[string]any{"type": orNull("string")}))
		}
	}
	Register(Spec{Out: "nself-yaml.v1.schema.json", Type: reflect.New(nselfYAMLType()).Elem().Interface(), Overrides: ovs})
}
