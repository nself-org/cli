package reconcile

import "strings"

// redactionSentinel chooses an internal marker absent from all input sides.
// The public [REDACTED] spelling remains ordinary input until final rendering.
func redactionSentinel(sides ...[]string) string {
	marker := "\x00redacted\x00"
	for {
		found := false
		for _, lines := range sides {
			if strings.Contains(strings.Join(lines, ""), marker) {
				found = true
				break
			}
		}
		if !found {
			return marker
		}
		marker += "\x00"
	}
}

// continueAfter opens continuation masking only for a span ending a literal
// block line. A closing quote or next flag on that line bounds the value.
func (sc *secretScan) continueAfter(line string, end int) {
	if sc.scalarIndent >= 0 && end == len(strings.TrimRight(line, " \t")) {
		sc.continuation = true
	}
}

// blockLine tracks YAML block scope and masks a previous secret's continuation
// through the next flag token. Dedenting ends the scalar; blank lines stay blank.
// It returns text for the ordinary span pass, preserving any following flag.
func (sc *secretScan) blockLine(line string) string {
	t := strings.TrimSpace(line)
	indent := len(line) - len(strings.TrimLeft(line, " \t"))
	if t != "" && indent <= sc.scalarIndent {
		sc.scalarIndent, sc.continuation = -1, false
	}
	if sc.scalarIndent < 0 {
		if m := yamlEntry.FindStringSubmatch(line); m != nil && blockScalar.MatchString(stripAnnotations(m[6])) {
			sc.scalarIndent = len(m[1]) + len(m[2])
		} else if strings.HasPrefix(t, "- ") && blockScalar.MatchString(stripAnnotations(strings.TrimPrefix(t, "- "))) {
			sc.scalarIndent = indent
		}
		return line
	}
	if !sc.continuation || t == "" {
		return line
	}
	end := len(line)
	if loc := flagToken.FindStringIndex(" " + line); loc != nil {
		end = max(0, loc[0])
		sc.continuation = false
	}
	prefix := strings.TrimRight(line[:end], " \t")
	if strings.TrimSpace(prefix) == "" {
		return line
	}
	sc.note(strings.TrimSpace(prefix))
	return line[:indent] + sc.marker + line[len(prefix):]
}
