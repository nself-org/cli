package cmdregistry

import "strings"

// parseArgs parses the positional arguments from a cobra Use string. The first
// word is the command name; after it, <x> is required, [x] is optional, a
// trailing "..." or "…" (inside or after the brackets) marks variadic, and the
// conventional [flags] marker is not an argument. Returns a non-nil slice.
func parseArgs(use string) []Arg {
	out := []Arg{}
	rest := strings.TrimSpace(use)
	if i := strings.IndexAny(rest, " \t"); i >= 0 {
		rest = rest[i+1:]
	} else {
		return out
	}
	for {
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" {
			return out
		}
		var tok string
		var required bool
		switch rest[0] {
		case '<':
			tok, rest = bracketed(rest, '<', '>')
			required = true
		case '[':
			tok, rest = bracketed(rest, '[', ']')
		default:
			i := strings.IndexAny(rest, " \t")
			if i < 0 {
				i = len(rest)
			}
			tok, rest = rest[:i], rest[i:]
			required = true
		}
		variadic := false
		for _, suf := range []string{"...", "…"} {
			if strings.HasPrefix(rest, suf) && (rest == suf || strings.ContainsAny(rest[len(suf):len(suf)+1], " \t")) {
				variadic = true
				rest = rest[len(suf):]
				break
			}
		}
		name := strings.TrimSpace(tok)
		for _, suf := range []string{"...", "…"} {
			if strings.HasSuffix(name, suf) {
				variadic = true
				name = strings.TrimSpace(strings.TrimSuffix(name, suf))
			}
		}
		if name == "" || name == "flags" || strings.HasPrefix(name, "-") {
			continue
		}
		out = append(out, Arg{Name: name, Required: required, Variadic: variadic})
	}
}

// bracketed returns the text inside the bracket pair that opens s, honouring
// nesting, and the remainder after the closing bracket. An unclosed bracket
// consumes the rest of the string.
func bracketed(s string, open, closeCh byte) (string, string) {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case open:
			depth++
		case closeCh:
			depth--
			if depth == 0 {
				return s[1:i], s[i+1:]
			}
		}
	}
	return s[1:], ""
}
