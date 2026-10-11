package config

import (
	"strings"
	"unicode"
)

// isJSONValue reports whether v is a JSON object or array value (starts with
// '{' or '['). These values must be quoted when written to .env files because
// the equals sign, braces, and inner quotes would otherwise be misinterpreted
// by naive line-by-line readers that do not implement the dotenv quoting spec.
func isJSONValue(v string) bool {
	s := strings.TrimSpace(v)
	return len(s) > 0 && (s[0] == '{' || s[0] == '[')
}

// QuoteEnvValue wraps v in single quotes when v is a JSON object or array
// value. Non-JSON values are returned unchanged. Single-quoted values are also
// returned unchanged (idempotent).
//
// Example:
//
//	QuoteEnvValue(`{"type":"HS256","key":"abc"}`) → `'{"type":"HS256","key":"abc"}'`
//	QuoteEnvValue("plain-value")                  → "plain-value"
//	QuoteEnvValue(`'already-quoted'`)             → `'already-quoted'`
func QuoteEnvValue(v string) string {
	if isSingleQuoted(v) {
		return v
	}
	if isJSONValue(v) {
		return "'" + v + "'"
	}
	return v
}

// UnquoteEnvValue strips a single layer of wrapping single quotes from v when
// the entire value is single-quoted. It is the inverse of QuoteEnvValue.
//
// Example:
//
//	UnquoteEnvValue(`'{"type":"HS256","key":"abc"}'`) → `{"type":"HS256","key":"abc"}`
//	UnquoteEnvValue("plain-value")                    → "plain-value"
func UnquoteEnvValue(v string) string {
	if isSingleQuoted(v) {
		return v[1 : len(v)-1]
	}
	return v
}

// isSingleQuoted reports whether v is wrapped in a matching pair of single
// quotes with at least one character between them.
func isSingleQuoted(v string) bool {
	return len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\''
}

// QuoteComposeEnvValue renders v as the value half of a docker compose
// --env-file line so that compose's dotenv parser reads back exactly v
// (D-0333). Unquoted compose dotenv values are trimmed, cut at " #",
// expanded on "$" and opened as quoted strings by a leading quote, so any
// value holding whitespace, '#', '$', a quote, a backtick or a backslash is
// quoted:
//
//   - Single quotes when v has no ' and no \: compose reads the bytes
//     between them verbatim.
//   - Double quotes otherwise: \ → \\, " → \", $ → $$. Compose unescapes \\
//     and \" and turns $$ into a literal $. Escaping ' inside single quotes
//     is not portable: compose before 2.24 (compose-go v1 dotenv) keeps the
//     backslash of \', and treats any backslash before the closing quote
//     as an escape.
//
// Verified byte for byte with `docker compose config` on 2.20.3, 2.24.0,
// 2.27.0 and 5.6.0. Unlike QuoteEnvValue it is not idempotent: a value that
// is already wrapped in quotes is a value with quotes in it and is quoted
// again.
//
// Example:
//
//	QuoteComposeEnvValue("plain-value") → plain-value
//	QuoteComposeEnvValue("$2a$10$abc")  → '$2a$10$abc'
//	QuoteComposeEnvValue("it's $5")     → "it's $$5"
//	QuoteComposeEnvValue(`a\b"c`)       → "a\\b\"c"
func QuoteComposeEnvValue(v string) string {
	if !strings.ContainsFunc(v, unicode.IsSpace) && !strings.ContainsAny(v, "#$'\"`\\") {
		return v
	}
	if !strings.ContainsAny(v, `'\`) {
		return "'" + v + "'"
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", "$$")
	return `"` + r.Replace(v) + `"`
}
