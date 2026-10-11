package report

// dotenv.go: a small, safe dotenv scanner used to learn names and values.
//
// Purpose: read KEY=VALUE pairs from env files without letting the body lines
// of a multi-line quoted value (a PEM key in .env) be mistaken for keys.
// Inputs: raw file text.
// Outputs: ordered pairs; a key is only ever an identifier.
// Constraints: pure function, no I/O. A line inside an open quote is value
// text, never a key. Unterminated quotes consume the rest of the file as the
// value (it is a scrub input, so over-reading is safe). An unquoted value that
// opens a PEM (-----BEGIN) runs through the -----END line; a bare BEGIN line
// does the same. Those lines are values (the pair with an empty key is a bare
// block): their base64 tail can look like KEY=VALUE and must never become a key.

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
		if strings.HasPrefix(t, pemBegin) { // a bare PEM block, no KEY=
			var end int
			t, end = pemRun(t, lines, i)
			i = end
			out = append(out, envPair{"", t})
			continue
		}
		k, v, ok := strings.Cut(t, "=")
		k = strings.TrimSpace(k)
		if !ok || !IsKeyName(k) {
			continue
		}
		v = strings.TrimSpace(v)
		if strings.HasPrefix(v, pemBegin) { // unquoted multi-line PEM value (M2)
			var end int
			v, end = pemRun(v, lines, i)
			i = end
			out = append(out, envPair{k, v})
			continue
		}
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

const pemBegin = "-----BEGIN"

// pemRun returns the PEM that starts with first (line i) and the index of the
// last line it spans: through the line holding -----END, or to the end of the
// text when the block is never closed (over-reading is safe).
func pemRun(first string, lines []string, i int) (string, int) {
	body := first
	for !strings.Contains(lines[i], "-----END") && i+1 < len(lines) {
		i++
		body += "\n" + strings.TrimSpace(lines[i])
	}
	return body, i
}
