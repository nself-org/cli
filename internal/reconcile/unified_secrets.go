package reconcile

// unified_secrets.go — secret redaction for the unified diff of non-env
// artifacts (D-0321). Split from unified.go for the file-size cap.
//
// Purpose: a docker-compose.yml, a JSON file or a shell snippet can hold a
// secret value (HASURA_GRAPHQL_ADMIN_SECRET in an environment map, a
// --requirepass flag in a command, a database URL with a password). The diff
// prints on stderr and in build --diff, so no such value may reach the text.
// Inputs: the artifact's lines, each with its newline if any. Outputs: the same
// number of lines with the values replaced by [REDACTED].
// Constraints: the only name rule is config.IsSecretKey; over-masking is the
// safe side. Redaction runs on the line text before any hunk is written.

import (
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/observability"
)

var (
	// yamlEntry matches a map entry: indent plus optional "- ", a bare or
	// quoted key, ":" and an optional value.
	yamlEntry = regexp.MustCompile(`^(\s*)((?:-\s+)?)(["']?)([A-Za-z_][A-Za-z0-9_.\-]*)(["']?)\s*:(?:\s+(.*))?$`)
	// blockScalar matches a YAML block scalar header (| or >, chomp and indent
	// indicators, optional comment).
	blockScalar = regexp.MustCompile(`^[|>][+-]?[0-9]?[+-]?\s*(?:#.*)?$`)
	// assignKey matches the "KEY=" of an assignment anywhere in a line.
	assignKey = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_.]*=`)
	// jsonPair matches a JSON "KEY": "value" pair.
	jsonPair = regexp.MustCompile(`("[A-Za-z_][A-Za-z0-9_.\-]*"\s*:\s*)"(?:[^"\\]|\\.)*"`)
	// secretFlag matches --name, then "=", whitespace or a quote-comma-quote
	// (a JSON/YAML list) and the value token or quoted string, if any.
	secretFlag = regexp.MustCompile(`(--[A-Za-z][A-Za-z0-9_-]*)(?:(=|[ \t]+|["']\s*,\s*["'])("[^"]*"|'[^']*'|[^\s"',\]]+))?`)
)

// redactSecretLines renders the lines of a non-env artifact so no secret value
// survives, tracking block scalars, multi-line quotes and split flags.
//
// Per line, in order: a YAML map entry with a secret key prints
// "<indent><key>: [REDACTED]" (a block scalar value redacts every deeper line;
// a quote that stays open redacts lines until it closes); then KEY=value
// assignments, JSON "KEY": "value" pairs and --flag value pairs with a secret
// name are masked in place; last, config.MaskURLCredentials and
// observability.Redact mask URL credentials and token shapes.
func redactSecretLines(lines []string) []string {
	out := make([]string, len(lines))
	block := -1       // key indent of an open block scalar, -1 when none
	quote := byte(0)  // open quote character of a multi-line value
	flagNext := false // previous line was a bare secret flag; this line is its value
	for i, line := range lines {
		nl := ""
		if strings.HasSuffix(line, "\n") {
			nl, line = "\n", strings.TrimSuffix(line, "\n")
		}
		t := strings.TrimSpace(line)
		ind := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		switch {
		case block >= 0 && (t == "" || len(ind) > block):
			out[i] = ind + redactedValue + nl
			continue
		case quote != 0:
			if closesQuote(line, quote) {
				quote = 0
			}
			out[i] = ind + redactedValue + nl
			continue
		case flagNext && t != "":
			flagNext = false
			dash := ""
			if strings.HasPrefix(t, "- ") {
				dash = "- "
			}
			out[i] = ind + dash + redactedValue + nl
			continue
		}
		block = -1
		text, nb, nq, nf := redactSecretLine(line)
		block, quote, flagNext = nb, nq, nf
		out[i] = observability.Redact(config.MaskURLCredentials(text)) + nl
	}
	return out
}

// redactSecretLine redacts one line (no newline). It returns the new text, the
// block-scalar key indent (-1 for none), the open quote (0 for none) and
// whether the line ended in a bare secret flag whose value is the next line.
func redactSecretLine(line string) (text string, block int, quote byte, flagNext bool) {
	block = -1
	if m := yamlEntry.FindStringSubmatch(line); m != nil && config.IsSecretKey(m[4]) {
		v := strings.TrimSpace(m[6])
		switch {
		case v == "":
		case blockScalar.MatchString(v):
			block = len(m[1])
			return m[1] + m[2] + m[3] + m[4] + m[5] + ": " + redactedValue, block, 0, false
		default:
			if v[0] == '"' || v[0] == '\'' {
				if !closesQuote(v[1:], v[0]) {
					quote = v[0]
				}
			}
			tail := ""
			if strings.HasSuffix(v, ",") && quote == 0 {
				tail = ","
			}
			return m[1] + m[2] + m[3] + m[4] + m[5] + ": " + redactedValue + tail, -1, quote, false
		}
	}
	text, quote = redactAssignments(line)
	text = jsonPair.ReplaceAllStringFunc(text, func(s string) string {
		k := jsonPair.FindStringSubmatch(s)
		name := strings.Trim(strings.TrimSpace(strings.SplitN(k[1], ":", 2)[0]), `"`)
		if config.IsSecretKey(name) {
			return k[1] + `"` + redactedValue + `"`
		}
		return s
	})
	text, flagNext = redactFlags(text)
	return text, -1, quote, flagNext
}

// redactAssignments masks the value of every KEY=value whose KEY is a secret
// name, keeping the quote and comma syntax around it. An unclosed quote
// returns its character so the following lines are redacted until it closes.
func redactAssignments(line string) (string, byte) {
	var sb strings.Builder
	pos, open := 0, byte(0)
	for _, loc := range assignKey.FindAllStringIndex(line, -1) {
		s, e := loc[0], loc[1]
		if s < pos || !config.IsSecretKey(line[s:e-1]) {
			continue
		}
		start, end, q := assignValueSpan(line, s, e)
		if end <= start {
			continue // empty value
		}
		sb.WriteString(line[pos:start])
		sb.WriteString(redactedValue)
		pos = end
		if q != 0 {
			open = q
			break
		}
	}
	sb.WriteString(line[pos:])
	return sb.String(), open
}

// assignValueSpan returns the span of the value of the assignment whose key
// spans line[s:e] (e is just past the "="), and the quote left open (0 if
// none). A value after a quote that opened before the key (- "KEY=value")
// ends at that quote; a quoted value spans its text between the quotes; an
// unquoted value ends at whitespace or a quote (and at a comma inside a
// [ ] or { } list), except after a line-leading KEY= where it ends the line.
func assignValueSpan(line string, s, e int) (start, end int, open byte) {
	if e >= len(line) {
		return e, e, 0
	}
	if s > 0 && (line[s-1] == '"' || line[s-1] == '\'') {
		end, open = closeAt(line, e, line[s-1])
		return e, end, open
	}
	if c := line[e]; c == '"' || c == '\'' {
		end, open = closeAt(line, e+1, c)
		return e + 1, end, open
	}
	// A line-leading assignment (KEY=v, - KEY=v, export KEY=v) owns the rest
	// of the line: a plain YAML or dotenv value may hold spaces and commas.
	switch strings.TrimSpace(line[:s]) {
	case "", "-", "export":
		return e, len(line), 0
	}
	delims := " \t\"'"
	if strings.ContainsAny(line[:s], "[{") {
		delims += ","
	}
	end = e
	for end < len(line) && !strings.ContainsRune(delims, rune(line[end])) {
		end++
	}
	return e, end, 0
}

// closeAt returns the offset of the first closing q at or after from (a
// backslash escapes in double quotes), or len(line) and q when it stays open.
func closeAt(line string, from int, q byte) (int, byte) {
	for i := from; i < len(line); i++ {
		if q == '"' && line[i] == '\\' {
			i++
			continue
		}
		if line[i] == q {
			return i, 0
		}
	}
	return len(line), q
}

// bareFlag matches a secret-capable flag that ends the line with no value.
var bareFlag = regexp.MustCompile(`(--[A-Za-z][A-Za-z0-9_-]*)["']?[\s,]*$`)

// redactFlags masks the value of every --name flag whose name is a secret
// name. It reports true when a secret flag ends the line with no value (a YAML
// list splits "--requirepass" and its value over two lines).
func redactFlags(line string) (string, bool) {
	out := secretFlag.ReplaceAllStringFunc(line, func(s string) string {
		m := secretFlag.FindStringSubmatch(s)
		if m[3] == "" || !config.IsSecretKey(m[1]) {
			return s
		}
		return m[1] + m[2] + redactedValue
	})
	m := bareFlag.FindStringSubmatch(out)
	return out, m != nil && config.IsSecretKey(m[1])
}
