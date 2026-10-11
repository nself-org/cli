package report

// compose_runtime.go: compose values in the spelling the container sees.
//
// Purpose: compose rewrites some strings before a container gets them: `$$`
// becomes `$`, and `${NAME:-default}`, `${NAME-default}` and `${NAME:+alt}`
// become their default or alternative. A log carries the runtime string, so a
// value learned from a compose file is also learned in that spelling (lead
// ruling R6 c). Nested defaults yield their innermost literal as well.
// Inputs: a key and the raw value text of a compose environment/args/labels
//         entry.
// Outputs: extra literals added to the input set under the owning key, and
//         under NAME when the default belongs to a variable.
// Constraints: pure string work; a form that still holds an unresolved ${...}
//         is not learned (it is not a runtime string).

import "strings"

// runtimeExpander expands one value and records every default/alternative.
type runtimeExpander struct {
	lits []namedLit
}

type namedLit struct{ name, text string }

// learnCompose learns a compose value in its file spelling and its runtime
// spellings.
func (in *inputSet) learnCompose(k, v string) {
	in.learn(k, v)
	if !strings.Contains(v, "$") {
		return
	}
	var e runtimeExpander
	forms := []string{strings.ReplaceAll(v, "$$", "$"), e.expand(v)}
	for _, f := range forms {
		if f != v && f != "" && !strings.Contains(f, "${") {
			in.learn(k, f)
		}
	}
	for _, l := range e.lits {
		if l.text != "" && !strings.Contains(l.text, "${") {
			in.learn(l.name, l.text)
			in.learn(k, l.text)
		}
	}
}

// expand returns s as compose would interpolate it with every variable unset
// (defaults taken) and records each default text.
func (e *runtimeExpander) expand(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); {
		switch {
		case s[i] != '$':
			sb.WriteByte(s[i])
			i++
		case i+1 < len(s) && s[i+1] == '$':
			sb.WriteByte('$')
			i += 2
		case i+1 < len(s) && s[i+1] == '{':
			end := closingBrace(s, i+2)
			if end < 0 {
				sb.WriteString(s[i:])
				return sb.String()
			}
			sb.WriteString(e.expandRef(s[i+2:end], s[i:end+1]))
			i = end + 1
		default:
			sb.WriteByte('$')
			i++
		}
	}
	return sb.String()
}

// expandRef expands the inside of one ${...}; raw is returned for a plain
// reference.
func (e *runtimeExpander) expandRef(inner, raw string) string {
	n := 0
	for n < len(inner) && (inner[n] == '_' || inner[n] >= '0' && inner[n] <= '9' || inner[n] >= 'a' && inner[n] <= 'z' || inner[n] >= 'A' && inner[n] <= 'Z') {
		n++
	}
	name, rest := inner[:n], inner[n:]
	for _, op := range []string{":-", ":+", "-", "+"} {
		if strings.HasPrefix(rest, op) {
			arg := e.expand(rest[len(op):])
			e.lits = append(e.lits, namedLit{name, arg})
			return arg
		}
	}
	return raw
}

// closingBrace returns the index of the } that closes a ${ whose body starts
// at from, counting nested braces, or -1.
func closingBrace(s string, from int) int {
	depth := 1
	for i := from; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return -1
}
