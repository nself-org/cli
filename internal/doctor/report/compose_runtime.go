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
// A compose value under a secret-named key that references variables
// (${NAME}, $NAME) is also interpolated against the env inputs already loaded
// (cascade layers, compose.env, .env.*, process env), and each referenced
// variable's value is learned, whatever the variable's own name (S-3).
// Outputs: extra literals added to the input set under the owning key, and
//         under NAME when the default belongs to a variable.
// Constraints: pure string work; a form that still holds an unresolved ${...}
//         is not learned (it is not a runtime string).

import "strings"

// runtimeExpander expands one value and records every default/alternative.
type runtimeExpander struct {
	lits []namedLit
	// env, when set, resolves variables: a plain reference becomes its value
	// (empty when unset) and the operators follow compose's rules. When nil
	// every variable is taken as unset and a plain reference stays as written.
	env func(name string) (string, bool)
}

type namedLit struct{ name, text string }

// learnCompose learns a compose value in its file spelling and its runtime
// spellings.
func (in *inputSet) learnCompose(k, v string) {
	in.fileSrc = true // a compose spelling: an unresolved ${REF} is no literal, a structure name may be dropped (R9)
	defer func() { in.fileSrc = false }()
	in.learn(k, v)
	if !strings.Contains(v, "$") {
		return
	}
	in.learnInterpolated(k, v)
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
		case e.env != nil && i+1 < len(s) && isNameByte(s[i+1], true):
			n := i + 1
			for n < len(s) && isNameByte(s[n], false) {
				n++
			}
			val, _ := e.env(s[i+1 : n])
			sb.WriteString(val)
			i = n
		default:
			sb.WriteByte('$')
			i++
		}
	}
	return sb.String()
}

// isNameByte reports whether c may appear in a variable name (first: may start one).
func isNameByte(c byte, first bool) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || !first && c >= '0' && c <= '9'
}

// expandRef expands the inside of one ${...}; raw is returned for a plain
// reference.
func (e *runtimeExpander) expandRef(inner, raw string) string {
	n := 0
	for n < len(inner) && isNameByte(inner[n], false) {
		n++
	}
	name, rest := inner[:n], inner[n:]
	for _, op := range []string{":-", ":+", "-", "+"} {
		if strings.HasPrefix(rest, op) {
			arg := e.expand(rest[len(op):])
			e.lits = append(e.lits, namedLit{name, arg})
			if e.env == nil {
				return arg
			}
			val, set := e.env(name)
			switch op {
			case ":-":
				if val != "" {
					return val
				}
			case "-":
				if set {
					return val
				}
			case ":+":
				if val == "" {
					return ""
				}
			case "+":
				if !set {
					return ""
				}
			}
			return arg
		}
	}
	if e.env != nil {
		val, _ := e.env(name)
		return val
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
