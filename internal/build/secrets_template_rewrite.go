package build

// Purpose: the line rewrites of TemplateSecrets. Phase A/A2 replace whole
//          "KEY: value" entries (rewriteKeyLines); Phase C replaces a secret
//          still literal after the env-key and URL phases by its ${VAR}
//          reference, without breaking the YAML, renaming a mapping key or
//          rewriting an existing reference. LiteralSecretLeaks
//          (secrets_template_leaks.go) reports whatever stays literal.
// Inputs:  compose YAML text, the sorted secret names, build.SecretEnvMap.
// Outputs: rewritten YAML text.
// Constraints: docker compose interpolates every string value, so a literal
//              "$" in a secret resolves differently from the ${VAR} consumers
//              (D-0635). A rewrite must leave the YAML loadable and every
//              resolved value unchanged; a raw byte replace may not cut
//              through a YAML escape ('' or \x).
// SPORT: cli/internal/build — secret templating (S-secret-env-templating).

import (
	"regexp"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// composeRefPattern matches the references TemplateSecrets and the compose
// generators emit: ${A}, ${A:-} and ${A:-${B}}.
var composeRefPattern = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*(?::-(?:\$\{[A-Za-z_][A-Za-z0-9_]*\})?)?\}`)

// envLinePattern matches a one-line "KEY: value" mapping entry.
var envLinePattern = regexp.MustCompile(`^(\s+)([A-Za-z_][A-Za-z0-9_]*): (.+)$`)

// blockScalarStart matches a line that opens a "|" or ">" block scalar,
// including an anchor or tag before it, an indentation or chomping
// indicator after it, and a trailing comment.
var blockScalarStart = regexp.MustCompile(`(?:^|:|-)\s*(?:(?:&\S+|!\S*)\s+)*[|>](?:[0-9][+-]?|[+-][0-9]?)?\s*(?:#.*)?$`)

// templateOtherLiterals is Phase C. Raw pass: a secret of 8+ characters that
// holds no quote, backslash or control character cannot overlap a YAML
// escape, so its raw occurrences become ${VAR}; the result is kept only when
// the YAML still loads (when it loaded before). Decoded pass: single-line
// "KEY: value" entries outside block scalars are decoded, so a secret the
// YAML encoder escaped (quotes, backslashes) is found and the line re-encoded.
// Neither pass rewrites text inside an existing ${...} reference.
func templateOtherLiterals(out string, keys []string, secrets map[string]string) string {
	shape, loads := yamlShape(out)
	for _, key := range keys {
		val := secrets[key]
		if len(val) < 8 || !rawReplaceSafe(val) {
			continue
		}
		next := replaceOutsideRefs(out, val, "${"+key+"}")
		if next == out {
			continue
		}
		// compose does not interpolate mapping keys: a secret that spells a
		// service or volume name must not rename it.
		if nextShape, ok := yamlShape(next); !loads || (ok && nextShape == shape) {
			out = next
		}
	}

	lines := strings.Split(out, "\n")
	blockIndent := -1
	for i, line := range lines {
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if blockIndent >= 0 {
			if strings.TrimSpace(line) == "" || indent > blockIndent {
				continue // inside a block scalar: literal text, not YAML
			}
			blockIndent = -1
		}
		if blockScalarStart.MatchString(line) {
			blockIndent = indent
			continue
		}
		if envLinePattern.MatchString(line) {
			lines[i] = templateDecodedLine(line, keys, secrets)
		}
	}
	next := strings.Join(lines, "\n")
	if nextShape, ok := yamlShape(next); loads && (!ok || nextShape != shape) {
		return out
	}
	return next
}

// rewriteKeyLines replaces the value of every "<indent>KEY: value" line for
// which fn returns a new value, and drops the lines indented deeper below
// it: they belong to the old value (a block scalar or a multi-line plain
// scalar), and kept under a one-line value they would extend it.
func rewriteKeyLines(out, key string, fn func(line string) (string, bool)) string {
	lines := strings.Split(out, "\n")
	kept := make([]string, 0, len(lines))
	dropBelow := -1
	var blanks []string // blank lines held until we know whose they are
	for _, line := range lines {
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if dropBelow >= 0 {
			if strings.TrimSpace(line) == "" {
				blanks = append(blanks, line)
				continue
			}
			if indent > dropBelow {
				blanks = blanks[:0] // inside the old value
				continue
			}
			dropBelow = -1
			kept = append(kept, blanks...)
			blanks = blanks[:0]
		}
		rest := line[indent:]
		if indent > 0 && strings.HasPrefix(rest, key+":") && len(rest) > len(key)+1 && unicode.IsSpace(rune(rest[len(key)+1])) {
			if v, ok := fn(line); ok {
				line = line[:indent] + key + ": " + v
				dropBelow = indent
			}
		}
		kept = append(kept, line)
	}
	kept = append(kept, blanks...)
	return strings.Join(kept, "\n")
}

// rawReplaceSafe reports whether val can be replaced byte for byte in YAML
// text: it holds no character the YAML encoder escapes or quotes around.
func rawReplaceSafe(val string) bool {
	for _, r := range val {
		if r == '\'' || r == '"' || r == '\\' || (r != ' ' && !unicode.IsPrint(r)) {
			return false
		}
	}
	return true
}

// replaceOutsideRefs replaces old with repl in s, except where a match cuts
// into a ${...} reference (starts or ends inside it without covering it), so
// a secret that spells part of a variable name cannot break a reference an
// earlier phase wrote. A secret that itself contains "${x}" still matches.
func replaceOutsideRefs(s, old, repl string) string {
	if old == "" {
		return s
	}
	refs := composeRefPattern.FindAllStringIndex(s, -1)
	cuts := func(start, end int) bool {
		for _, r := range refs {
			overlaps := start < r[1] && end > r[0]
			covers := start <= r[0] && end >= r[1]
			if overlaps && !covers {
				return true
			}
		}
		return false
	}
	var sb strings.Builder
	last := 0
	for i := 0; i+len(old) <= len(s); {
		j := strings.Index(s[i:], old)
		if j < 0 {
			break
		}
		start := i + j
		end := start + len(old)
		if cuts(start, end) {
			i = start + 1
			continue
		}
		sb.WriteString(s[last:start])
		sb.WriteString(repl)
		last, i = end, end
	}
	sb.WriteString(s[last:])
	return sb.String()
}

// yamlShape returns the mapping keys of text in document order (the part
// of a compose file docker compose does not interpolate) and whether text
// parses as YAML at all.
func yamlShape(text string) (string, bool) {
	var root yaml.Node
	if yaml.Unmarshal([]byte(text), &root) != nil {
		return "", false
	}
	var sb strings.Builder
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		for i, c := range n.Content {
			if n.Kind == yaml.MappingNode && i%2 == 0 {
				sb.WriteString(c.Value)
				sb.WriteByte(0)
			}
			walk(c)
		}
	}
	walk(&root)
	return sb.String(), true
}

// templateDecodedLine returns line with every secret (8+ characters) in its
// YAML-decoded value replaced by ${KEY}, re-encoded as YAML. Lines that do
// not decode to a single string, or hold no secret, are returned unchanged.
func templateDecodedLine(line string, keys []string, secrets map[string]string) string {
	m := envLinePattern.FindStringSubmatch(line)
	var decoded map[string]string
	if yaml.Unmarshal([]byte(m[2]+": "+m[3]), &decoded) != nil || len(decoded) != 1 {
		return line
	}
	v, ok := decoded[m[2]]
	if !ok {
		return line
	}
	nv := v
	for _, key := range keys {
		if val := secrets[key]; len(val) >= 8 {
			nv = replaceOutsideRefs(nv, val, "${"+key+"}")
		}
	}
	if nv == v {
		return line
	}
	b, err := yaml.Marshal(map[string]string{m[2]: nv})
	enc := strings.TrimSuffix(string(b), "\n")
	if err != nil || strings.Contains(enc, "\n") {
		return line // a block scalar would need re-indenting; keep the line
	}
	return m[1] + enc
}

// lineValueIs reports whether the "KEY: value" YAML line decodes to val. A
// value holding a quote or a backslash is escaped in the line, so a substring
// check misses it and the literal stayed in the file (D-0635).
func lineValueIs(line, val string) bool {
	var m map[string]string
	if err := yaml.Unmarshal([]byte(strings.TrimSpace(line)), &m); err != nil {
		return false
	}
	for _, v := range m {
		return v == val
	}
	return false
}
