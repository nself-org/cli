package exporter

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Purpose: derive every text form a secret can take in pg_restore output.
// A value is first wrapped by zero to three container escapes (jsonb/json
// text, array element, record or hstore element), then rendered by one of
// three terminals: verbatim (DDL text), COPY text (data blocks) or an SQL
// literal (COMMENT, defaults). A jsonb value inside a text[] column is
// array-escaped JSON, and COPY then doubles every backslash again, so the
// layers must compose rather than stand alone.
// Inputs: one raw secret. Outputs: distinct byte patterns.
// Constraints: depth is capped at three layers; duplicates are dropped so an
// ordinary secret costs a handful of patterns, not hundreds.

// containerLayers are the Postgres output escapes that can wrap a value.
var containerLayers = []func(string) string{jsonText, arrayElement, recordElement}

// jsonText escapes as json_out/jsonb_out do: quote, backslash, \b \f \n \r \t
// and other control characters as \u00XX. Nothing else is touched.
func jsonText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// arrayElement escapes a quoted array or hstore element: quote and backslash
// gain a backslash.
func arrayElement(s string) string {
	return strings.NewReplacer(`"`, `\"`, `\`, `\\`).Replace(s)
}

// recordElement escapes a quoted composite (record) field: quote and
// backslash are doubled.
func recordElement(s string) string {
	return strings.NewReplacer(`"`, `""`, `\`, `\\`).Replace(s)
}

// copyText escapes as COPY text format does.
func copyText(s string) string {
	return strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\r", "\\r", "\t", "\\t", "\b", "\\b", "\f", "\\f", "\v", "\\v").Replace(s)
}

// sqlLiteral escapes a single-quoted SQL literal under standard_conforming_strings.
func sqlLiteral(s string) string { return strings.ReplaceAll(s, "'", "''") }

// secretForms returns the distinct renderings of one secret.
func secretForms(s string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(f string) {
		if f != "" && !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	layers := []string{s}
	frontier := layers
	for depth := 0; depth < 3; depth++ {
		var next []string
		for _, f := range frontier {
			for _, wrap := range containerLayers {
				next = append(next, wrap(f))
			}
		}
		layers = append(layers, next...)
		frontier = next
	}
	for _, f := range layers {
		add(f)
		add(copyText(f))
		add(sqlLiteral(f))
		add(sqlLiteral(copyText(f)))
	}
	return out
}

// clientJSONForms returns the spellings an application chooses when it
// serializes JSON itself and Postgres stores the text verbatim (json columns,
// text columns holding JSON): "/" written as "\/" (PHP json_encode), and every
// non-alphanumeric ASCII byte written as \u00xx in lower- or upper-case hex
// (.NET and others). Each is returned raw, in COPY text and as an SQL literal;
// the set is deduplicated by the caller and skips the raw form itself.
func clientJSONForms(s string) []string {
	variants := []string{
		strings.ReplaceAll(s, "/", `\/`),
		strings.ReplaceAll(jsonText(s), "/", `\/`),
		uEscapeASCII(s, "%04x"),
		uEscapeASCII(s, "%04X"),
	}
	var out []string
	for _, v := range variants {
		if v == s {
			continue
		}
		out = append(out, v, copyText(v), sqlLiteral(v), sqlLiteral(copyText(v)))
	}
	return out
}

// uEscapeASCII writes every ASCII byte that is not a letter or digit as
// \u00XX using the given hex verb; other runes pass through unchanged.
func uEscapeASCII(s, verb string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x80 && (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z'):
			fmt.Fprintf(&b, `\u`+verb, r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Purpose: match raw values and every encoded representation without false
// negatives when a secret spans read boundaries. Inputs: resolved values.
// Outputs: pattern set. Constraints: all patterns have at least six raw bytes.
func secretPatterns(secrets []string) [][]byte {
	seen := map[string]bool{}
	var p [][]byte
	add := func(f string) {
		if f != "" && !seen[f] {
			seen[f] = true
			p = append(p, []byte(f))
		}
	}
	for _, s := range secrets {
		if len(s) < 6 {
			continue
		}
		for _, f := range secretForms(s) {
			add(f)
		}
		add(strings.TrimPrefix(url.UserPassword("", s).String(), ":"))
		add(url.QueryEscape(s))
		for _, f := range clientJSONForms(s) {
			add(f)
		}
		add(hex.EncodeToString([]byte(s)))
		// encoding/json also escapes <, > and & (as <); apps that
		// serialize with it store that spelling, and a second JSON layer
		// covers a JSON document stored inside a JSON string.
		b, _ := json.Marshal(s)
		add(string(b[1 : len(b)-1]))
		add(copyText(string(b[1 : len(b)-1])))
		b, _ = json.Marshal(string(b[1 : len(b)-1]))
		add(string(b[1 : len(b)-1]))
		add(copyText(string(b[1 : len(b)-1])))
	}
	return p
}
