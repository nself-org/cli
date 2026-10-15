package build

// Purpose: the value rewrites of TemplateSecrets. Phase A/A2 replace whole
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
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// composeRefPattern matches the references TemplateSecrets and the compose
// generators emit: ${A}, ${A:-} and ${A:-${B}}.
var composeRefPattern = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*(?::-(?:\$\{[A-Za-z_][A-Za-z0-9_]*\})?)?\}`)

// templateOtherLiterals is Phase C: walk decoded value scalars, then splice
// only changed tokens back into the original text. Keys and existing refs
// are excluded. Re-parsing verifies the complete tree before accepting edits.
func templateOtherLiterals(out string, keys []string, secrets map[string]string) string {
	var root yaml.Node
	if yaml.Unmarshal([]byte(out), &root) != nil {
		return out
	}
	var edits []scalarEdit
	var walk func(*yaml.Node, int, bool)
	walk = func(n *yaml.Node, indent int, flow bool) {
		flow = flow || n.Style&yaml.FlowStyle != 0
		switch n.Kind {
		case yaml.ScalarNode:
			value := n.Value
			for _, key := range keys {
				if val := secrets[key]; len(val) >= 8 {
					value = replaceOutsideRefs(value, val, "${"+key+"}")
				}
			}
			if value != n.Value {
				edits = append(edits, encodeScalarEdit(out, n, value, indent, flow))
				n.Value, n.Tag = value, "!!str"
			}
		case yaml.MappingNode:
			for i := 1; i < len(n.Content); i += 2 {
				walk(n.Content[i], n.Content[i-1].Column-1, flow)
			}
		default:
			for _, c := range n.Content {
				walk(c, n.Column-1, flow)
			}
		}
	}
	walk(&root, -1, false)
	if len(edits) == 0 {
		return out
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	next := out
	for _, edit := range edits {
		next = next[:edit.start] + edit.text + next[edit.end:]
	}
	var check yaml.Node
	if yaml.Unmarshal([]byte(next), &check) != nil || !sameScalarTree(&root, &check) {
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
