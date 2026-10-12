package config

// validator_env_dollar.go — finds '$' in env-file values that the loader expands.
//
// Purpose: godotenv expands $NAME in unquoted and double-quoted values. A
//          bcrypt hash or a password written as ADMIN_PASSWORD_HASH=$2a$10$...
//          loads as a different string with no error (D-0552). This check names
//          the file, line and key so the owner can single-quote the value.
// Inputs:  the project dir and environment name (cascade files that exist), or
//          one file's name and bytes (scanUnquotedDollar).
// Outputs: DollarFinding values. A finding never holds the value or any part of
//          it: it is printed to a terminal.
// Constraints: the rule mirrors godotenv v1.5.1's expandVariables regex
//          `(\\)?(\$)(\()?\{?([A-Z0-9_]+)?\}?`. A '$' is rewritten only when an
//          optional '(' and '{' are followed by at least one [A-Z0-9_]; a '$'
//          after a backslash is an escape. `${NAME}` is a deliberate
//          reference and is not flagged. Single-quoted values are never
//          expanded. A lowercase `$name` or `$(cmd)` is left alone by godotenv,
//          so it is not flagged either.
// SPORT:   cli/internal/config — D-0552.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// DollarFinding is one value line holding a '$' the loader would expand.
type DollarFinding struct {
	File string // cascade file name, e.g. ".env.secrets"
	Line int    // 1-based line of the '$'
	Key  string
}

// UnquotedDollarFindings scans each existing file of env's cascade.
func UnquotedDollarFindings(projectDir, env string) []DollarFinding {
	var out []DollarFinding
	for _, name := range EnvCascadeOrder(env, LegacyOrderActive()) {
		data, err := os.ReadFile(filepath.Join(projectDir, name))
		if err != nil {
			continue
		}
		out = append(out, scanUnquotedDollar(name, data)...)
	}
	return out
}

// scanUnquotedDollar reports each line of data whose unquoted or double-quoted
// value holds a '$' that godotenv would expand. Pure: no file access.
func scanUnquotedDollar(name string, data []byte) []DollarFinding {
	var out []DollarFinding
	lines := strings.Split(string(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))), "\n")
	var open byte // quote of a multi-line value still open, 0 when none
	var openKey string
	for i, line := range lines {
		var text, key string
		var expands bool // false for single-quoted text
		if open != 0 {
			end := closingQuote(line, 0, open)
			text, key, expands = line, openKey, open == '"'
			if end >= 0 {
				text, open = line[:end], 0
			}
		} else {
			k, v, ok := splitEnvLine(line)
			if !ok {
				continue
			}
			key, expands = k, true
			if v != "" && (v[0] == '"' || v[0] == '\'') {
				q := v[0]
				expands = q == '"'
				if end := closingQuote(v, 1, q); end >= 0 {
					text = v[1:end]
				} else {
					text, open, openKey = v[1:], q, k
				}
			} else {
				text = stripInlineComment(v)
			}
		}
		if expands && hasExpandedDollar(text) {
			out = append(out, DollarFinding{File: name, Line: i + 1, Key: key})
		}
	}
	return out
}

// splitEnvLine splits "KEY=value", "export KEY=value" or "KEY: value" into the
// key and the value with leading blanks removed. ok is false for blank lines,
// comments and lines without a plausible key.
func splitEnvLine(line string) (key, value string, ok bool) {
	s := strings.TrimLeft(line, " \t")
	if s == "" || s[0] == '#' {
		return "", "", false
	}
	if rest := strings.TrimPrefix(s, "export"); rest != s && rest != "" && (rest[0] == ' ' || rest[0] == '\t') {
		s = strings.TrimLeft(rest, " \t")
	}
	sep := strings.IndexAny(s, "=:")
	if sep <= 0 {
		return "", "", false
	}
	key = strings.TrimRight(s[:sep], " \t")
	for _, c := range key {
		if !(c == '_' || c == '.' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return "", "", false
		}
	}
	return key, strings.TrimLeft(s[sep+1:], " \t"), true
}

// closingQuote returns the index of the first q at or after from that is not
// preceded by a backslash (godotenv's terminator rule), or -1.
func closingQuote(s string, from int, q byte) int {
	for i := from; i < len(s); i++ {
		if s[i] == q && (i == 0 || s[i-1] != '\\') {
			return i
		}
	}
	return -1
}

// stripInlineComment drops a trailing ` # comment` the way godotenv does: the
// last '#' that follows a blank.
func stripInlineComment(v string) string {
	for i := len(v) - 1; i > 0; i-- {
		if v[i] == '#' && (v[i-1] == ' ' || v[i-1] == '\t') {
			return v[:i]
		}
	}
	return v
}

// hasExpandedDollar reports whether s holds a '$' that godotenv's
// expandVariables rewrites and that is not a deliberate ${NAME} reference.
func hasExpandedDollar(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '$' || (i > 0 && s[i-1] == '\\') {
			continue
		}
		j := i + 1
		paren := j < len(s) && s[j] == '('
		if paren {
			j++
		}
		brace := j < len(s) && s[j] == '{'
		if brace {
			j++
		}
		k := j
		for k < len(s) && (s[k] == '_' || s[k] >= '0' && s[k] <= '9' || s[k] >= 'A' && s[k] <= 'Z') {
			k++
		}
		if k == j {
			continue // no name: godotenv leaves it alone
		}
		if !paren && brace && k < len(s) && s[k] == '}' {
			continue // ${NAME}: a deliberate reference
		}
		return true
	}
	return false
}
