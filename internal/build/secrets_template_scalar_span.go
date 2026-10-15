package build

// Purpose: byte-stable scalar re-encoding for Phase C secret templating.
// Inputs: parsed scalar positions and the original YAML source.
// Outputs: edits covering only scalar tokens, retaining tags and anchors.
// Constraints: yaml.v3 columns count runes; edits must pass a full tree check.
// SPORT: cli/internal/build — secret templating (S-secret-env-templating).

import (
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

type scalarEdit struct {
	start, end int
	text       string
}

// encodeScalarEdit re-encodes one value, retaining the surrounding source.
func encodeScalarEdit(src string, n *yaml.Node, value string, indent int, flow bool) scalarEdit {
	start := scalarOffset(src, n.Line, n.Column)
	start = skipScalarProperties(src, start)
	end, suffix := scalarEnd(src, start, indent, flow)
	encoded := yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	if flow || strings.ContainsAny(value, "\r\n") {
		encoded.Style = yaml.DoubleQuotedStyle
	}
	b, err := yaml.Marshal(&encoded)
	if err != nil {
		return scalarEdit{start: start, end: end, text: src[start:end]}
	}
	return scalarEdit{start: start, end: end, text: strings.TrimSuffix(string(b), "\n") + suffix}
}

// scalarOffset converts yaml.v3's one-based rune position to a byte offset.
func scalarOffset(src string, line, column int) int {
	i := 0
	for l := 1; l < line; l++ {
		i += strings.IndexByte(src[i:], '\n') + 1
	}
	for c := 1; c < column && i < len(src); c++ {
		_, size := utf8.DecodeRuneInString(src[i:])
		i += size
	}
	return i
}

// skipScalarProperties leaves tags and anchors outside the edited token.
func skipScalarProperties(src string, i int) int {
	for i < len(src) && (src[i] == '&' || src[i] == '!') {
		for i < len(src) && !strings.ContainsRune(" \t\r\n", rune(src[i])) {
			i++
		}
		for i < len(src) && strings.ContainsRune(" \t\r\n", rune(src[i])) {
			i++
		}
	}
	return i
}

// scalarEnd finds the token boundary for quoted, block and plain scalars.
func scalarEnd(src string, start, indent int, flow bool) (int, string) {
	if start == len(src) {
		return start, ""
	}
	switch src[start] {
	case '|', '>':
		return blockScalarEnd(src, start, indent)
	case '\'', '"':
		quote := src[start]
		for i := start + 1; i < len(src); i++ {
			if quote == '"' && src[i] == '\\' {
				i++
			} else if src[i] == quote {
				if quote == '\'' && i+1 < len(src) && src[i+1] == '\'' {
					i++
					continue
				}
				return i + 1, ""
			}
		}
	}
	return plainScalarEnd(src, start, indent, flow), ""
}

// plainScalarEnd includes indented continuation lines, excluding comments
// and flow delimiters. The full tree check rejects any ambiguous boundary.
func plainScalarEnd(src string, start, indent int, flow bool) int {
	i := start
	for ; i < len(src); i++ {
		if flow && strings.ContainsRune(",[]{}", rune(src[i])) {
			break
		}
		if src[i] == '#' && (i == start || strings.ContainsRune(" \t\r\n", rune(src[i-1]))) {
			break
		}
		if src[i] == '\n' && !flow {
			next := i + 1
			for next < len(src) && src[next] == ' ' {
				next++
			}
			if next == len(src) || (src[next] != '\n' && src[next] != '\r' && next-i-1 <= indent) {
				break
			}
		}
	}
	return start + len(strings.TrimRight(src[start:i], " \t\r\n"))
}

// blockScalarEnd consumes the scalar body, preserving its header comment
// and line ending beside the new quoted value. Chomping is encoded in value.
func blockScalarEnd(src string, start, indent int) (int, string) {
	headerEnd := strings.IndexByte(src[start:], '\n')
	if headerEnd < 0 {
		return len(src), ""
	}
	headerEnd += start
	indicatorEnd := start + 1
	contentIndent := 0
	for indicatorEnd < headerEnd && strings.ContainsRune("123456789+-", rune(src[indicatorEnd])) {
		if c := src[indicatorEnd]; c >= '1' && c <= '9' {
			contentIndent = max(indent, 0) + int(c-'0')
		}
		indicatorEnd++
	}
	end := headerEnd + 1
	for end < len(src) {
		next := strings.IndexByte(src[end:], '\n')
		if next < 0 {
			next = len(src)
		} else {
			next += end + 1
		}
		line := strings.TrimRight(src[end:next], "\r\n")
		spaces := len(line) - len(strings.TrimLeft(line, " "))
		if strings.TrimSpace(line) != "" {
			if contentIndent == 0 {
				contentIndent = spaces
			}
			if spaces <= indent || spaces < contentIndent {
				break
			}
		}
		end = next
	}
	return end, src[indicatorEnd : headerEnd+1]
}

// sameScalarTree verifies all keys, values, tags and alias names after edits.
// Style and comments are source presentation, not the decoded YAML tree.
func sameScalarTree(a, b *yaml.Node) bool {
	if a.Kind != b.Kind || a.Tag != b.Tag || a.Value != b.Value || a.Anchor != b.Anchor || len(a.Content) != len(b.Content) {
		return false
	}
	for i := range a.Content {
		if !sameScalarTree(a.Content[i], b.Content[i]) {
			return false
		}
	}
	return true
}
