package report

// dotenv.go: a small, safe dotenv scanner used to learn names and values.
//
// Purpose: read KEY=VALUE pairs from env files without letting the body lines
// of a multi-line quoted value (a PEM key in .env) be mistaken for keys.
// Inputs: raw file text.
// Outputs: ordered pairs; a key is only ever an identifier.
// Constraints: pure function, no I/O. A line inside an open quote is value
// text, never a key. Unterminated quotes consume the rest of the file as the
// value (it is a scrub input, so over-reading is safe).

import (
	"regexp"
	"strings"
)

// keyRe is what a config key name may look like (M1). Anything else is dropped
// and never written to config-keys.txt.
var keyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\-]*$`)

// IsKeyName reports whether s is a valid config key name.
func IsKeyName(s string) bool { return keyRe.MatchString(s) }

type envPair struct{ Key, Value string }

// scanDotenv returns the pairs of a dotenv text.
func scanDotenv(raw string) []envPair {
	var out []envPair
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		t = strings.TrimSpace(strings.TrimPrefix(t, "export "))
		k, v, ok := strings.Cut(t, "=")
		k = strings.TrimSpace(k)
		if !ok || !IsKeyName(k) {
			continue
		}
		v = strings.TrimSpace(v)
		if v != "" && (v[0] == '"' || v[0] == '\'') {
			q := v[0]
			body, closed := closeQuote(v[1:], q)
			for !closed && i+1 < len(lines) {
				i++
				var more string
				more, closed = closeQuote(lines[i], q)
				body += "\n" + more
			}
			out = append(out, envPair{k, body})
			continue
		}
		out = append(out, envPair{k, v})
	}
	return out
}

// closeQuote returns s up to the first unescaped q and whether q was found.
func closeQuote(s string, q byte) (string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && q == '"' {
			i++
			continue
		}
		if s[i] == q {
			return s[:i], true
		}
	}
	return s, false
}
