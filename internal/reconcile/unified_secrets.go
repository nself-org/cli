package reconcile

// unified_secrets.go — secret redaction for the unified diff of non-env
// artifacts (D-0321). unified_secret_spans.go holds the in-line span rules.
//
// Purpose: a docker-compose.yml, a JSON file or a shell snippet can hold a
// secret value (HASURA_GRAPHQL_ADMIN_SECRET in an environment map, a
// --requirepass flag in a command, a database URL with a password). The diff
// prints on stderr and in build --diff, so no such value may reach the text.
// Inputs: the artifact's lines, each with its newline if any. Outputs: the same
// number of lines with the values replaced by [REDACTED].
// Constraints: the only name rule is config.IsSecretKey; over-masking is the
// safe side. Redaction runs on the line text before any hunk is written. Two
// passes: scanSecretLines redacts by key, flag and pair and collects the values
// it hid; applySecretValues then hides every other literal copy of those values
// (a healthcheck that repeats the --requirepass password). Unified collects
// from both sides of the diff before applying. observability.Redact is not used
// here: it masks IPs, ports and digit runs, which would
// hide a port change in a compose file.

import (
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// minSecretValueLen is the shortest collected value that is replaced
// everywhere else in the file; shorter ones would mask ordinary words.
const minSecretValueLen = 4

var (
	// yamlEntry matches a map entry: indent plus optional "- ", a bare or
	// quoted key, ":" and an optional value.
	yamlEntry = regexp.MustCompile(`^(\s*)((?:-\s+)?)(["']?)([A-Za-z_][A-Za-z0-9_.\-]*)(["']?)\s*:(?:\s+(.*))?$`)
	// blockScalar matches a YAML block scalar header (| or >, chomp and indent
	// indicators, optional comment).
	blockScalar = regexp.MustCompile(`^[|>][+-]?[0-9]?[+-]?\s*(?:#.*)?$`)
)

// secretScan collects the secret values hidden during one pass.
type secretScan struct {
	vals         []string
	literal      bool   // the open scope is a block scalar: its lines are literal text
	marker       string // inserted redactions, absent from every original input side
	scalarIndent int    // column of a YAML block scalar header, -1 outside a scalar
	continuation bool   // a secret span reached the end of a block scalar line
}

// note records a hidden value: quotes and spaces trimmed, at least
// minSecretValueLen long, not a boolean or null literal, not a redaction.
func (sc *secretScan) note(v string) {
	v = strings.Trim(strings.TrimSpace(v), `"'`)
	switch strings.ToLower(v) {
	case "true", "false", "null":
		return
	}
	if len(v) >= minSecretValueLen && !strings.Contains(v, sc.marker) {
		sc.vals = append(sc.vals, v)
	}
}

// stripComment cuts a trailing comment from plain text: a # at the start or
// after a space or tab, outside quotes. A quote opens only at the start or
// after whitespace (an apostrophe inside a word is text) and a backslash
// escapes inside double quotes.
func stripComment(v string) string {
	var q byte
	for i := 0; i < len(v); i++ {
		c := v[i]
		atWord := i == 0 || v[i-1] == ' ' || v[i-1] == '\t'
		switch {
		case q != 0:
			if c == '\\' && q == '"' {
				i++
			} else if c == q {
				q = 0
			}
		case (c == '"' || c == '\'') && atWord:
			q = c
		case c == '#' && atWord:
			return strings.TrimSpace(v[:i])
		}
	}
	return strings.TrimSpace(v)
}

// stripAnnotations removes leading &anchor and !tag tokens from a YAML value.
func stripAnnotations(v string) string {
	v = strings.TrimSpace(v)
	for v != "" && (v[0] == '&' || v[0] == '!') {
		i := strings.IndexAny(v, " \t")
		if i < 0 {
			return ""
		}
		v = strings.TrimSpace(v[i:])
	}
	return v
}

// scalarText returns the text of a YAML scalar value for collection: no
// leading &anchor or !tag, no block indicator, quotes removed, and for a plain
// value no trailing " # comment". An unclosed quote yields the rest of v.
func scalarText(v string) string {
	v = stripAnnotations(v)
	if v == "" || blockScalar.MatchString(v) {
		return ""
	}
	if v[0] == '"' || v[0] == '\'' {
		end, _ := closeAt(v, 1, v[0])
		return v[1:end]
	}
	return stripComment(v)
}

// redactSecretLines renders the lines of one non-env artifact so no secret
// value survives (scan, then replace the collected values in the same lines).
func redactSecretLines(lines []string) []string {
	marker := redactionSentinel(lines)
	out, vals := scanSecretLines(lines, marker)
	return applySecretValues(out, vals, marker)
}

// redactSecretSides redacts the before and after lines of one artifact. The
// values hidden on either side are replaced on both, so a secret that changed
// does not survive as the old copy on the other side.
func redactSecretSides(al, bl []string) ([]string, []string) {
	marker := redactionSentinel(al, bl)
	ao, av := scanSecretLines(al, marker)
	bo, bv := scanSecretLines(bl, marker)
	vals := append(av, bv...)
	return applySecretValues(ao, vals, marker), applySecretValues(bo, vals, marker)
}

// scanSecretLines is the first pass: it returns the lines with secret values
// hidden and the values it hid. Per line, in order: a YAML map entry with a
// secret key prints "<indent><key>: [REDACTED]" and opens a scope (a block
// scalar, a value that continues on deeper lines, or a quote that stays open
// redacts those lines); then --flag values, then KEY=value and KEY: value pairs
// anywhere in the line are masked in place; last, config.MaskURLCredentials.
func scanSecretLines(lines []string, marker string) ([]string, []string) {
	sc := &secretScan{marker: marker, scalarIndent: -1}
	out := make([]string, len(lines))
	block := -1       // key column of an open secret scope, -1 when none
	quote := byte(0)  // open quote character of a multi-line value
	flagNext := false // previous line was a bare secret flag; this line is its value
	for i, line := range lines {
		nl := ""
		if strings.HasSuffix(line, "\n") {
			nl, line = "\n", strings.TrimSuffix(line, "\n")
		}
		if quote != 0 {
			sc.continuation = false // the quote pass must see the original closing quote
		}
		line = sc.blockLine(line)
		t := strings.TrimSpace(line)
		ind := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		skip := t == "" || strings.HasPrefix(t, "#")
		switch {
		case quote != 0:
			if end, q := closeAt(line, 0, quote); q == 0 {
				quote = 0
				sc.note(line[:end]) // text up to the closing quote; a comment after it is not part of the value
			} else {
				sc.note(t)
			}
			out[i] = ind + sc.marker + nl
			continue
		case block >= 0 && (t == "" || len(ind) > block):
			if text := strings.TrimPrefix(t, "- "); sc.literal {
				sc.note(text)
			} else {
				sc.note(stripComment(text))
			}
			if t == "" {
				out[i] = line + nl // a blank line stays blank
			} else {
				out[i] = ind + sc.marker + nl
			}
			continue
		case flagNext && !skip:
			flagNext = false
			dash := ""
			if strings.HasPrefix(t, "- ") {
				dash = "- "
				t = strings.TrimPrefix(t, "- ")
			}
			sc.note(stripComment(t))
			out[i] = ind + dash + sc.marker + nl
			sc.continuation = sc.scalarIndent >= 0
			continue
		}
		text, nb, nq, nf := sc.line(line)
		block, quote, flagNext = nb, nq, nf || (flagNext && skip)
		out[i] = config.MaskURLCredentials(text) + nl
	}
	return out, sc.vals
}

// line redacts one line (no newline). It returns the new text, the key column
// of a scope that deeper lines belong to (-1 for none), the open quote (0 for
// none) and whether the line ended in a bare secret flag.
func (sc *secretScan) line(line string) (text string, block int, quote byte, flagNext bool) {
	if m := yamlEntry.FindStringSubmatch(line); m != nil && m[3] == m[5] && config.IsSecretKey(m[4]) {
		return sc.secretEntry(line, m)
	}
	line = sc.directive(line)
	text, quote = sc.flags(line)
	flagNext = endsInSecretFlag(text)
	text, q := sc.pairs(text)
	if quote == 0 {
		quote = q
	}
	return text, -1, quote, flagNext
}

// secretEntry redacts a YAML map entry whose key is a secret name (m is the
// yamlEntry match of line; a quote before the key with none after is the
// opening quote of a scalar, not a quoted key, and goes to pairs instead). Any such entry opens a scope at its key column, so
// a value that continues on deeper lines is hidden too (safe side).
func (sc *secretScan) secretEntry(line string, m []string) (string, int, byte, bool) {
	col := len(m[1]) + len(m[2])
	sc.literal = false
	head := m[1] + m[2] + m[3] + m[4] + m[5]
	v := strings.TrimSpace(m[6])
	switch {
	case v == "" || v[0] == '#':
		return line, col, 0, false // nothing to hide on this line
	case v[0] == '&' || v[0] == '!' || blockScalar.MatchString(v):
		sc.note(scalarText(v))
		sc.literal = blockScalar.MatchString(strings.TrimSpace(stripAnnotations(v)))
		return head + ": " + sc.marker, col, 0, false
	}
	var open byte
	if v[0] == '"' || v[0] == '\'' {
		_, open = closeAt(v, 1, v[0])
	}
	sc.note(scalarText(v))
	tail := ""
	if strings.HasSuffix(v, ",") && open == 0 {
		tail = ","
	}
	return head + ": " + sc.marker + tail, col, open, false
}
