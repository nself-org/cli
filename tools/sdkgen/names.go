package main

// Purpose: derive TypeScript identifiers from OpenAPI names.
// Inputs: operationIds, component names, property keys. Outputs: valid identifiers.
// Constraints: deterministic; collisions are reported, never renamed silently.

import (
	"fmt"
	"regexp"
	"strings"
)

var identRe = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

// words splits snake, kebab and dotted names into lowercase-first words.
func words(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == '_' || r == '-' || r == '.' || r == '/' })
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// pascal turns config_get into ConfigGet.
func pascal(s string) string {
	var b strings.Builder
	for _, w := range words(s) {
		b.WriteString(upperFirst(w))
	}
	return b.String()
}

// camel turns config_get into configGet.
func camel(s string) string {
	p := pascal(s)
	if p == "" {
		return p
	}
	return strings.ToLower(p[:1]) + p[1:]
}

// methodName maps nself_config_get to configGet.
func methodName(operationID string) string {
	return camel(strings.TrimPrefix(operationID, "nself_"))
}

// typeName maps a component name to its exported TypeScript type.
func typeName(component string) string {
	switch component {
	case "error":
		return "ErrorDetail"
	case "confirmation":
		return "Confirmation"
	}
	return pascal(strings.TrimPrefix(component, "nself_"))
}

// propKey quotes a property key when it is not a plain identifier.
func propKey(k string) string {
	if identRe.MatchString(k) {
		return k
	}
	return fmt.Sprintf("%q", k)
}

// comment makes a one-line JSDoc body that cannot end the comment early.
func comment(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.ReplaceAll(s, "*/", "*\u200b/")
}
