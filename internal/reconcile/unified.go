package reconcile

import (
	"fmt"
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
// as KEY=[REDACTED] and any other line passes through observability.Redact,
// before the line is written, so no value reaches the text. When the change
// is too large for an edit script (over maxTraceDistance changed lines) the
// text is the two header lines and a one-line notice.
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
		fmt.Fprintf(&sb, "@@ diff too large to display (%d lines changed) @@\n", d)
		return sb.String()
	}
	show := func(l string) string { return l }
	if IsEnvPath(path) {
		show = redactEnvLine
	}
	writeHunks(&sb, ops, al, bl, show)
	return sb.String()
}

// redactEnvLine renders one env-file line (with its newline, if any) so that
// no value survives: KEY=value becomes KEY=[REDACTED]; anything else goes
// through observability.Redact.
func redactEnvLine(line string) string {
	nl := ""
	if strings.HasSuffix(line, "\n") {
		nl, line = "\n", strings.TrimSuffix(line, "\n")
	}
	t := strings.TrimSpace(line)
	if t == "" {
		return line + nl
	}
	if i := strings.Index(t, "="); i >= 0 {
		name := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t[:i]), "export "))
		return observability.Redact(name+"="+redactedValue) + nl
	}
	return observability.Redact(t) + nl
}

// writeHunks groups ops into hunks with unifiedContext lines of context and
// writes them. Lines missing a final newline get the standard marker.
func writeHunks(sb *strings.Builder, ops []editOp, al, bl []string, show func(string) string) {
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
		writeHunk(sb, ops[start:stop], al, bl, show)
		i = stop
	}
}

// writeHunk writes one "@@ -a,n +b,m @@" block.
func writeHunk(sb *strings.Builder, ops []editOp, al, bl []string, show func(string) string) {
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
		sb.WriteString(show(line))
		if !strings.HasSuffix(line, "\n") {
			sb.WriteString("\n\\ No newline at end of file\n")
		}
	}
}
