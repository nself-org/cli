package report

// scrub_block.go: YAML block scalars under secret-named keys (M2).
//
// Purpose: `SOME_TOKEN: |` followed by indented lines holds a multi-line
// secret; the assignment pass only sees the indicator, so the body lines
// would survive. This pass redacts them.
// Inputs: text.
// Outputs: text whose block bodies are [REDACTED] lines (indent kept).
// Constraints: the body is every following line indented deeper than the key
// (blank lines are kept); counts one assignment per redacted body line.

import (
	"regexp"
	"strings"
)

var blockKeyRe = regexp.MustCompile(`(?i)^([ \t]*(?:-[ \t]+)?)["']?` + secretIdent + `["']?[ \t]*:[ \t]*[|>][+\-0-9]*[ \t]*(?:#.*)?$`)

func (s *Scrubber) scrubBlockScalars(text string) string {
	if !strings.ContainsAny(text, "|>") {
		return text
	}
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		m := blockKeyRe.FindStringSubmatch(strings.TrimRight(lines[i], "\r"))
		if m == nil {
			continue
		}
		keyCol := len(m[1])
		last := i
		for j := i + 1; j < len(lines); j++ {
			body := strings.TrimRight(lines[j], "\r")
			trimmed := strings.TrimLeft(body, " \t")
			if trimmed == "" {
				continue
			}
			indent := len(body) - len(trimmed)
			if indent <= keyCol {
				break
			}
			last = j
			if !onlyPlaceholders(strings.TrimRight(trimmed, " \t")) {
				lines[j] = body[:indent] + placeholder
				s.counts[ClassAssignment]++
			}
		}
		i = last
	}
	return strings.Join(lines, "\n")
}
