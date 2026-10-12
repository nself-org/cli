package main

// Purpose: map JSON Schema nodes to TypeScript type expressions.
// Inputs: Schema nodes. Outputs: type strings; object bodies are inlined.
// Constraints: unsupported shapes become `unknown` (never `any`).

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const refPrefix = "#/components/schemas/"

func isFalse(raw json.RawMessage) bool { return strings.TrimSpace(string(raw)) == "false" }

func isTrue(raw json.RawMessage) bool { return strings.TrimSpace(string(raw)) == "true" }

// schemaTypes returns the declared types ("type" may be a string or a list).
func schemaTypes(s *Schema) []string {
	if len(s.Type) == 0 {
		return nil
	}
	var one string
	if json.Unmarshal(s.Type, &one) == nil {
		return []string{one}
	}
	var many []string
	_ = json.Unmarshal(s.Type, &many)
	return many
}

// tsType renders the expression for s; indent is the nesting depth of objects.
func tsType(s *Schema, depth int) string {
	if s == nil {
		return "unknown"
	}
	if s.Ref != "" {
		return typeName(strings.TrimPrefix(s.Ref, refPrefix))
	}
	if len(s.Const) > 0 {
		return string(s.Const)
	}
	if len(s.Enum) > 0 {
		parts := make([]string, len(s.Enum))
		for i, e := range s.Enum {
			parts[i] = string(e)
		}
		return strings.Join(parts, " | ")
	}
	for _, c := range []struct {
		list []*Schema
		sep  string
	}{{s.OneOf, " | "}, {s.AnyOf, " | "}, {s.AllOf, " & "}} {
		if len(c.list) > 0 {
			parts := make([]string, len(c.list))
			for i, sub := range c.list {
				parts[i] = "(" + tsType(sub, depth) + ")"
			}
			return strings.Join(parts, c.sep)
		}
	}
	types := schemaTypes(s)
	if len(types) == 0 {
		return "unknown"
	}
	parts := make([]string, 0, len(types))
	for _, t := range types {
		parts = append(parts, tsKind(s, t, depth))
	}
	return strings.Join(parts, " | ")
}

func tsKind(s *Schema, kind string, depth int) string {
	switch kind {
	case "string":
		return "string"
	case "integer", "number":
		return "number"
	case "boolean":
		return "boolean"
	case "null":
		return "null"
	case "array":
		return tsArray(s, depth)
	case "object":
		return tsObject(s, depth)
	}
	return "unknown"
}

func tsArray(s *Schema, depth int) string {
	rest := ""
	if len(s.Items) > 0 && !isFalse(s.Items) {
		var item Schema
		if isTrue(s.Items) || json.Unmarshal(s.Items, &item) != nil {
			rest = "unknown"
		} else {
			rest = tsType(&item, depth)
		}
	}
	if len(s.PrefixItems) == 0 {
		if rest == "" {
			return "[]"
		}
		return "Array<" + rest + ">"
	}
	min := len(s.PrefixItems)
	if s.MinItems != nil && *s.MinItems < min {
		min = *s.MinItems
	}
	parts := make([]string, 0, len(s.PrefixItems)+1)
	for i, p := range s.PrefixItems {
		opt := ""
		if i >= min {
			opt = "?"
		}
		parts = append(parts, tsType(p, depth)+opt)
	}
	if rest != "" {
		parts = append(parts, "..."+"Array<"+rest+">")
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func tsObject(s *Schema, depth int) string {
	if len(s.Properties) == 0 {
		switch {
		case len(s.AdditionalProperties) == 0 || isTrue(s.AdditionalProperties):
			return "Record<string, unknown>"
		case isFalse(s.AdditionalProperties):
			return "Record<string, never>"
		}
		var v Schema
		if json.Unmarshal(s.AdditionalProperties, &v) != nil {
			return "Record<string, unknown>"
		}
		return "Record<string, " + tsType(&v, depth) + ">"
	}
	req := map[string]bool{}
	for _, r := range s.Required {
		req[r] = true
	}
	keys := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pad := strings.Repeat("  ", depth+1)
	var b strings.Builder
	b.WriteString("{\n")
	for _, k := range keys {
		p := s.Properties[k]
		if p.Description != "" {
			fmt.Fprintf(&b, "%s/** %s */\n", pad, comment(p.Description))
		}
		opt := "?"
		if req[k] {
			opt = ""
		}
		fmt.Fprintf(&b, "%s%s%s: %s;\n", pad, propKey(k), opt, tsType(p, depth+1))
	}
	if len(s.AdditionalProperties) > 0 && !isFalse(s.AdditionalProperties) {
		fmt.Fprintf(&b, "%s[key: string]: unknown;\n", pad)
	}
	b.WriteString(strings.Repeat("  ", depth) + "}")
	return b.String()
}
