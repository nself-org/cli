// Package policyroles reads the role names that CREATE POLICY ... TO lists
// name in schema-only SQL (pg_dump text), for the restore paths in
// internal/backup (P7-PROD-88).
//
// Purpose: `pg_restore --no-owner --no-acl` still runs CREATE POLICY ... TO
// <role>, which fails when <role> is missing; the caller creates the roles
// Collect returns, NOLOGIN, before the restore.
// Inputs: schema-only SQL text. Outputs: validated role names.
// Constraints: nothing is executed here. Only names matching NamePattern are
// returned; any other name is an error wrapping ErrBackupRestoreFailed.
package policyroles

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// NamePattern is the only role-name form Collect accepts.
const NamePattern = `^[a-z_][a-z0-9_]{0,62}$`

var (
	policyRoleRe   = regexp.MustCompile(NamePattern)
	createPolicyRe = regexp.MustCompile(`(?i)^CREATE\s+POLICY\s`)
)

// policyRoleKeywords are TO-list entries that are not roles to create.
var policyRoleKeywords = map[string]bool{"public": true, "current_user": true, "session_user": true, "current_role": true}

const maxSchemaBytes = 256 << 20 // bound on the schema text read from the archive

// Collect reads schema-only SQL (pg_dump text) and returns the
// sorted, de-duplicated role names of every CREATE POLICY ... TO list, minus
// PUBLIC, CURRENT_USER, SESSION_USER and CURRENT_ROLE. A name outside
// policyRoleRe is an error wrapping ErrBackupRestoreFailed. Statements are
// split on top-level semicolons, so text inside comments, string literals and
// dollar-quoted bodies is never read as a policy.
func Collect(r io.Reader) ([]string, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxSchemaBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read schema: %v", errs.ErrBackupRestoreFailed, err)
	}
	if len(b) > maxSchemaBytes {
		return nil, fmt.Errorf("%w: schema text over %d bytes", errs.ErrBackupRestoreFailed, maxSchemaBytes)
	}
	seen := map[string]bool{}
	for _, stmt := range sqlStatements(string(b)) {
		if !createPolicyRe.MatchString(stmt) {
			continue
		}
		roles, err := policyToRoles(stmt)
		if err != nil {
			return nil, err
		}
		for _, role := range roles {
			seen[role] = true
		}
	}
	out := make([]string, 0, len(seen))
	for role := range seen {
		out = append(out, role)
	}
	sort.Strings(out)
	return out, nil
}

// policyToRoles returns the validated roles of one CREATE POLICY statement's
// TO list (none when it has no TO clause).
func policyToRoles(stmt string) ([]string, error) {
	toks := sqlTokens(stmt)
	i := 0
	for ; i < len(toks); i++ {
		if !toks[i].quoted && (toks[i].text == ";" || strings.EqualFold(toks[i].text, "USING") || strings.EqualFold(toks[i].text, "WITH")) {
			return nil, nil
		}
		if !toks[i].quoted && strings.EqualFold(toks[i].text, "TO") {
			break
		}
	}
	var roles []string
	for i++; i < len(toks); i += 2 {
		tok := toks[i]
		name := tok.text
		if !tok.quoted {
			name = strings.ToLower(name)
			if policyRoleKeywords[name] {
				name = ""
			}
		}
		if name != "" {
			if !policyRoleRe.MatchString(name) {
				return nil, fmt.Errorf("%w: policy role %q is not a plain role name (^[a-z_][a-z0-9_]{0,62}$); create it by hand and restore again", errs.ErrBackupRestoreFailed, name)
			}
			roles = append(roles, name)
		}
		if i+1 >= len(toks) || toks[i+1].quoted || toks[i+1].text != "," {
			break
		}
	}
	return roles, nil
}

// sqlStatements splits SQL on top-level semicolons, skipping comments, '...'
// and E'...' literals, "..." identifiers, $tag$ bodies and psql meta-command
// lines (\restrict); leading whitespace and comments are trimmed.
func sqlStatements(s string) []string {
	var out []string
	start, atStart := 0, true
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case atStart && c == '\\':
			i = skipPast(s, i, "\n")
		case strings.HasPrefix(s[i:], "--"):
			i = skipPast(s, i, "\n")
		case strings.HasPrefix(s[i:], "/*"):
			i = skipPast(s, i+2, "*/") // pg_dump never nests these
		case c == ';':
			out = append(out, s[start:i])
			i, atStart = i+1, true
		default:
			switch tag, ok := dollarTag(s, i); {
			case c == '\'':
				i = skipQuoted(s, i, '\'', i > 0 && (s[i-1] == 'E' || s[i-1] == 'e') && (i < 2 || !isWordByte(s[i-2])))
			case c == '"':
				i = skipQuoted(s, i, '"', false)
			case ok:
				i = skipPast(s, i+len(tag), tag)
			default:
				i++
			}
			atStart = false
		}
		if atStart {
			start = i
		}
	}
	return out
}

// skipPast returns the index just after the first delim at or after from.
func skipPast(s string, from int, delim string) int {
	if k := strings.Index(s[from:], delim); k >= 0 {
		return from + k + len(delim)
	}
	return len(s)
}

// skipQuoted skips a q-quoted run at i (qq escapes q; so does \ if backslash).
func skipQuoted(s string, i int, q byte, backslash bool) int {
	for j := i + 1; j < len(s); j++ {
		switch {
		case backslash && s[j] == '\\':
			j++
		case s[j] == q && j+1 < len(s) && s[j+1] == q:
			j++
		case s[j] == q:
			return j + 1
		}
	}
	return len(s)
}

// dollarTag returns the $tag$ delimiter that starts at i, if one does.
func dollarTag(s string, i int) (string, bool) {
	if s[i] != '$' || (i > 0 && isWordByte(s[i-1])) {
		return "", false
	}
	j := i + 1
	for j < len(s) && (s[j] == '_' || s[j] >= 'a' && s[j] <= 'z' || s[j] >= 'A' && s[j] <= 'Z' || j > i+1 && s[j] >= '0' && s[j] <= '9' || s[j] >= 0x80) {
		j++
	}
	if j < len(s) && s[j] == '$' {
		return s[i : j+1], true
	}
	return "", false
}

type sqlToken struct {
	text   string
	quoted bool // a "double-quoted" identifier, text unescaped
}

// sqlTokens splits SQL into words, "quoted identifiers" and punctuation, up to
// the first string literal (none comes before a policy's TO list).
func sqlTokens(s string) []sqlToken {
	var toks []sqlToken
	for i := 0; i < len(s); {
		ch := s[i]
		switch {
		case ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r':
			i++
		case ch == '\'':
			return toks
		case ch == '"':
			j := skipQuoted(s, i, '"', false)
			toks = append(toks, sqlToken{text: strings.ReplaceAll(strings.TrimSuffix(s[i+1:j], `"`), `""`, `"`), quoted: true})
			i = j
		case isWordByte(ch):
			j := i
			for j < len(s) && isWordByte(s[j]) {
				j++
			}
			toks = append(toks, sqlToken{text: s[i:j]})
			i = j
		default:
			toks = append(toks, sqlToken{text: string(ch)})
			i++
		}
	}
	return toks
}

func isWordByte(c byte) bool {
	return c == '_' || c == '$' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}
