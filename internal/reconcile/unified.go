package reconcile

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/observability"
)

// unifiedContext is the number of unchanged lines shown around each hunk.
const unifiedContext = 3

// redactedValue replaces the value of an env line in diff text.
const redactedValue = "[REDACTED]"

// Unified returns a unified diff of one artifact for `--diff` output.
//
// Inputs: the artifact path and the before/after bytes (nil for an absent
// side). Outputs: "--- a/<path>", "+++ b/<path>" and hunks with three lines of
// context, or "" when the sides are equal or either exceeds MaxDiffLines.
// Env-kind paths (IsEnvPath) show KEY names only: every KEY=value line prints
// as KEY=[REDACTED], and the continuation lines of a multi-line or quoted value
// print as [REDACTED] (see redactEnvLines), before the line is written, so no
// value reaches the text. When the change is too large for an edit script (over
// maxTraceDistance changed lines, or maxEditDistance) the text is the two
// header lines and a one-line notice.
func Unified(path string, a, b []byte) string {
	if string(a) == string(b) || countLines(a) > MaxDiffLines || countLines(b) > MaxDiffLines {
		return ""
	}
	al, bl := splitLines(a), splitLines(b)
	x, y := intern(al, bl)
	d, ops := myers(x, y, true)
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- a/%s\n+++ b/%s\n", path, path)
	if ops == nil {
		if d < 0 {
			sb.WriteString("@@ diff too large to display @@\n")
		} else {
			fmt.Fprintf(&sb, "@@ diff too large to display (%d lines changed) @@\n", d)
		}
		return sb.String()
	}
	if IsEnvPath(path) {
		al, bl = redactEnvLines(al), redactEnvLines(bl)
	}
	writeHunks(&sb, ops, al, bl)
	return sb.String()
}

// envKeyLine matches the start of a dotenv assignment: optional `export`, a
// key, then "=".
var envKeyLine = regexp.MustCompile(`^\s*(export\s+)?([A-Za-z_][A-Za-z0-9_.]*)\s*=(.*)$`)

// redactEnvLines renders env-file lines (each with its newline, if any) so no
// value survives, in order, tracking quotes across lines.
//
// A KEY=value line becomes KEY=[REDACTED] (export dropped). A value that opens
// a quote it does not close on the same line (a PEM key in "..." or '...'), or
// that ends in a backslash, continues on the next lines: every continuation
// line is printed as [REDACTED] until the quote closes. A line that is neither
// blank, a comment, nor an assignment and is not inside a value prints as
// [REDACTED], because it cannot be told apart from a value fragment. Comments
// go through observability.Redact (a comment holding "=" prints its text as
// "<text before =>=[REDACTED]").
func redactEnvLines(lines []string) []string {
	out := make([]string, len(lines))
	quote := byte(0) // open quote character, 0 when not inside a value
	cont := false    // previous value line ended in a backslash
	for i, line := range lines {
		nl := ""
		if strings.HasSuffix(line, "\n") {
			nl, line = "\n", strings.TrimSuffix(line, "\n")
		}
		t := strings.TrimSpace(line)
		switch {
		case quote != 0:
			if closesQuote(line, quote) {
				quote = 0
			}
			out[i] = redactedValue + nl
		case cont:
			cont = strings.HasSuffix(t, "\\")
			out[i] = redactedValue + nl
		case t == "":
			out[i] = line + nl
		case strings.HasPrefix(t, "#"):
			out[i] = redactComment(t) + nl
		default:
			m := envKeyLine.FindStringSubmatch(line)
			if m == nil {
				out[i] = redactedValue + nl
				continue
			}
			out[i] = observability.Redact(m[2]+"="+redactedValue) + nl
			val := strings.TrimSpace(m[3])
			if val != "" && (val[0] == '"' || val[0] == '\'') && !closesQuote(val[1:], val[0]) {
				quote = val[0]
			} else if strings.HasSuffix(val, "\\") {
				cont = true
			}
		}
	}
	return out
}

// closesQuote reports whether s contains the closing quote q (a double quote
// preceded by a backslash does not close).
func closesQuote(s string, q byte) bool {
	for i := 0; i < len(s); i++ {
		if q == '"' && s[i] == '\\' {
			i++
			continue
		}
		if s[i] == q {
			return true
		}
	}
	return false
}

// redactComment prints a comment line without any value: text before "=" is
// kept with the value replaced, other text goes through observability.Redact.
func redactComment(t string) string {
	if i := strings.Index(t, "="); i >= 0 {
		return observability.Redact(strings.TrimSpace(t[:i]) + "=" + redactedValue)
	}
	return observability.Redact(t)
}

// writeHunks groups ops into hunks with unifiedContext lines of context and
// writes them. Lines missing a final newline get the standard marker.
func writeHunks(sb *strings.Builder, ops []editOp, al, bl []string) {
	i := 0
	for i < len(ops) {
		if ops[i].Tag == '=' {
			i++
			continue
		}
		start := max(i-unifiedContext, 0)
		end := i
		for j := i; j < len(ops); {
			if ops[j].Tag != '=' {
				end = j + 1
				j++
				continue
			}
			k := j
			for k < len(ops) && ops[k].Tag == '=' {
				k++
			}
			if k == len(ops) || k-j > 2*unifiedContext {
				break
			}
			j = k
		}
		stop := min(end+unifiedContext, len(ops))
		writeHunk(sb, ops[start:stop], al, bl)
		i = stop
	}
}

// writeHunk writes one "@@ -a,n +b,m @@" block.
func writeHunk(sb *strings.Builder, ops []editOp, al, bl []string) {
	var an, bn int
	for _, op := range ops {
		if op.Tag != '+' {
			an++
		}
		if op.Tag != '-' {
			bn++
		}
	}
	as, bs := ops[0].A+1, ops[0].B+1
	if an == 0 {
		as--
	}
	if bn == 0 {
		bs--
	}
	fmt.Fprintf(sb, "@@ -%d,%d +%d,%d @@\n", as, an, bs, bn)
	for _, op := range ops {
		var line string
		if op.Tag == '+' {
			line = bl[op.B]
		} else {
			line = al[op.A]
		}
		tag := op.Tag
		if tag == '=' {
			tag = ' '
		}
		sb.WriteByte(tag)
		sb.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			sb.WriteString("\n\\ No newline at end of file\n")
		}
	}
}
