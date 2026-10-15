package config

import (
	"regexp"
	"strings"
)

// Purpose: mask schemeless DSN userinfo without changing email/user@host values.
// Inputs: a connection value; outputs: masked value and whether it matched.
// Constraints: a username and colon must precede the last @; URLs use their
// existing masking path. SPORT: D-0743.
func maskSchemelessUserinfo(v string) (string, bool) {
	colon, at := strings.IndexByte(v, ':'), strings.LastIndexByte(v, '@')
	if colon <= 0 || at <= colon || at+1 == len(v) || strings.HasPrefix(v[colon+1:], "//") {
		return v, false
	}
	if strings.ContainsAny(v[:colon], " \t\r\n\f=@/") || isConninfoSpace(v[at+1]) {
		return v, false
	}
	return "****" + v[at:], true
}

var conninfoPassword = regexp.MustCompile(`(?i)(^|[\t\n\r\f ])(?:password|pwd)[\t ]*=[\t ]*`)

// Purpose: mask keyword connection passwords, including quoted/escaped values.
// Inputs: a value; outputs: the same string with password/pwd values masked.
// Constraints: recognize whole keyword names; consume unterminated quotes to
// the end rather than exposing a partial password. SPORT: D-0743.
func maskConninfoCredentials(v string) string {
	var b strings.Builder
	done := 0
	for _, match := range conninfoPassword.FindAllStringIndex(v, -1) {
		if match[0] < done {
			continue
		}
		end := conninfoValueEnd(v, match[1])
		b.WriteString(v[done:match[1]])
		b.WriteString("****")
		done = end
	}
	if done == 0 {
		return v
	}
	b.WriteString(v[done:])
	return b.String()
}

// Purpose: locate a libpq-style value boundary without interpreting its secret.
// Inputs: string and value offset; outputs: first offset after the value.
// Constraints: backslashes escape the next byte, including quotes and spaces;
// malformed quotes consume the remainder. SPORT: D-0743.
func conninfoValueEnd(v string, start int) int {
	var quote byte
	i := start
	if i < len(v) && (v[i] == '\'' || v[i] == '"') {
		quote = v[i]
		i++
	}
	for i < len(v) {
		switch {
		case v[i] == '\\' && i+1 < len(v):
			i += 2
		case quote != 0 && v[i] == quote:
			return i + 1
		case quote == 0 && isConninfoSpace(v[i]):
			return i
		default:
			i++
		}
	}
	return i
}

// isConninfoSpace reports the ASCII whitespace accepted between conninfo pairs.
func isConninfoSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == '\v'
}
