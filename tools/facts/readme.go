// Purpose: fill and verify the `<!-- BEGIN GENERATED:facts:<key> -->` blocks
// of a README from the facts document.
// Inputs: README text and model.Facts. Outputs: the filled text.
// Constraints: a README with no facts blocks is returned unchanged. A block
// whose marker sits alone on its line is a block (content on its own lines);
// otherwise it is inline (content between the markers on the same line).
// Unknown keys, a missing END marker and a nested BEGIN are errors. Other
// GENERATED blocks are ignored.
package main

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/tools/facts/model"
)

const (
	beginPrefix = "<!-- BEGIN GENERATED:facts:"
	endPrefix   = "<!-- END GENERATED:facts:"
	markerTail  = " -->"
)

// ReadmeKeys are the supported block keys.
var ReadmeKeys = []string{"version", "verbs", "top-level"}

// blockText returns the generated text for one key.
func blockText(key string, f model.Facts) (string, error) {
	switch key {
	case "version":
		return f.Version, nil
	case "verbs":
		q := make([]string, len(f.Verbs))
		for i, v := range f.Verbs {
			q[i] = "`" + v + "`"
		}
		return strings.Join(q, ", "), nil
	case "top-level":
		s := fmt.Sprintf("%d top-level commands", f.Counts.TopLevel)
		if belowV15(f.Version) {
			s += " in " + f.ContractMode + " mode, `NSELF_V15=1`"
		}
		return s, nil
	}
	return "", fmt.Errorf("unknown facts block %q (want one of %s)", key, strings.Join(ReadmeKeys, ", "))
}

// FillReadme returns text with every facts block regenerated.
func FillReadme(text string, f model.Facts) (string, error) {
	var out strings.Builder
	rest := text
	for {
		i := strings.Index(rest, beginPrefix)
		if i < 0 {
			out.WriteString(rest)
			return out.String(), nil
		}
		afterPrefix := rest[i+len(beginPrefix):]
		j := strings.Index(afterPrefix, markerTail)
		if j < 0 {
			return "", fmt.Errorf("unterminated BEGIN marker near %q", clip(rest[i:]))
		}
		key := afterPrefix[:j]
		body, err := blockText(key, f)
		if err != nil {
			return "", err
		}
		begin := rest[i : i+len(beginPrefix)+j+len(markerTail)]
		tail := afterPrefix[j+len(markerTail):]
		end := endPrefix + key + markerTail
		k := strings.Index(tail, end)
		if k < 0 {
			return "", fmt.Errorf("facts block %q has no END marker", key)
		}
		if strings.Contains(tail[:k], beginPrefix) {
			return "", fmt.Errorf("facts block %q is not closed before the next BEGIN", key)
		}
		lineStart := i == 0 || rest[i-1] == '\n'
		lineEnd := strings.HasPrefix(tail, "\n") || strings.HasPrefix(tail, "\r\n")
		out.WriteString(rest[:i])
		out.WriteString(begin)
		if lineStart && lineEnd {
			out.WriteString("\n" + body + "\n")
		} else {
			out.WriteString(body)
		}
		out.WriteString(end)
		rest = tail[k+len(end):]
	}
}

func clip(s string) string {
	if len(s) > 40 {
		return s[:40]
	}
	return s
}
