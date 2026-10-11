package build

// Purpose: treats the legacy literal external network `nself_network` as the
// project network. v1.5 folds it into the one network the main compose
// defines: the external definition is deleted and service references to it
// become ${DOCKER_NETWORK}.
// Inputs: the fragment bytes, their parsed top-level mapping, the plugin name.
// Outputs: line edits for normalizeComposeNetworks to apply.
// Constraints: byte-splice edits only (comments, quoting and layout survive).
// Why not keep the definition and add `name: ${DOCKER_NETWORK}`: an external
// network is never created by `docker compose up`, so the first start of a
// fresh project fails with "declared as external, but could not be found".
// A service that lists the network in mapping form (`nself_network:` with
// settings such as aliases) cannot be rewritten: compose does not interpolate
// mapping keys, so `${DOCKER_NETWORK}:` is rejected by its schema. Any shape
// that cannot be edited line-locally (a multi-line flow definition, a mapping
// form reference, a service that already lists the project network, a
// reference column that does not line up) is E129 in v1.5, raised by the
// caller from the reason returned here. v1.4 rewrites nothing;
// foreignExternalNetworks already stops reporting the literal in both modes.
// What is rewritten is decided on the resolved view (plugins_network_resolved.go)
// and checked again on the output (legacyNetworkRemains).

import (
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/compat"
)

// legacyProjectNetwork is the literal external network name older fragments
// (access-controls, forgejo and others) use for the project network.
const legacyProjectNetwork = "nself_network"

// projectNetworkName is what service references to the legacy network become.
const projectNetworkName = "${DOCKER_NETWORK}"

// legacyNetworkEdits plans the v1.5 fold of the legacy network (see the file
// header). keys are the legacy network keys found; a non-empty why means the
// fold cannot be done safely (the caller raises E129).
func legacyNetworkEdits(content []byte, top *yaml.Node, res *resolvedFragment) (edits []lineEdit, keys map[string]bool, why string) {
	// compat.V15(P7-PLUG-17): external network nself_network keeps its literal name -> its definition is dropped and service references use ${DOCKER_NETWORK}
	if !compat.V15() {
		return nil, nil, ""
	}
	keys = res.legacyKeys()
	if len(keys) == 0 {
		return nil, nil, ""
	}
	nets := yamlGet(top, "networks")
	if nets == nil || nets.Kind != yaml.MappingNode {
		return nil, keys, "its definition comes from an alias or merge key"
	}
	lines := strings.Split(string(content), "\n")
	var legacy []*yaml.Node // key nodes written in this fragment
	for i := 0; i+1 < len(nets.Content); i += 2 {
		if keys[nets.Content[i].Value] {
			legacy = append(legacy, nets.Content[i])
		}
	}
	if len(legacy) != len(keys) {
		return nil, keys, "its definition comes from an alias or merge key"
	}
	for i := 0; i+1 < len(top.Content); i += 2 {
		if len(legacy)*2 == len(nets.Content) && top.Content[i+1] == nets { // the only entries: drop networks: itself
			e, ok := dropBlock(top.Content[i], nets, lines)
			if !ok {
				return nil, keys, "its definition is a multi-line flow mapping"
			}
			edits = append(edits, e)
		}
	}
	if len(edits) == 0 && nets.Style&yaml.FlowStyle != 0 {
		return nil, keys, "networks is a flow mapping that holds other networks"
	}
	if len(edits) == 0 {
		for i := 0; i+1 < len(nets.Content); i += 2 {
			if !keys[nets.Content[i].Value] {
				continue
			}
			e, ok := dropBlock(nets.Content[i], nets.Content[i+1], lines)
			if !ok {
				return nil, keys, "its definition is a multi-line flow mapping"
			}
			edits = append(edits, e)
		}
	}
	if svcs := yamlGet(top, "services"); svcs != nil && svcs.Kind == yaml.MappingNode {
		for i := 1; i < len(svcs.Content); i += 2 {
			refs, why := legacyRefEdits(svcs.Content[i], keys, lines)
			if why != "" {
				return nil, keys, why
			}
			edits = append(edits, refs...)
		}
	}
	return edits, keys, ""
}

// legacyNetworkRemains reports whether the rewritten fragment still has a
// legacy network definition or a service reference to one of keys (for
// instance one supplied through an alias or merge key).
func legacyNetworkRemains(out []byte, keys map[string]bool) bool {
	res, err := resolveFragment(out)
	if err != nil {
		return true
	}
	if len(res.legacyKeys()) > 0 {
		return true
	}
	for _, s := range res.Services {
		if s.refersTo(keys) {
			return true
		}
	}
	return false
}

// legacyRefEdits plans rewriting one service's references to the legacy keys;
// a non-empty reason means the service cannot be rewritten safely.
func legacyRefEdits(svc *yaml.Node, keys map[string]bool, lines []string) ([]lineEdit, string) {
	nets := yamlGet(svc, "networks")
	if nets == nil {
		return nil, ""
	}
	if nets.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(nets.Content); i += 2 {
			if keys[nets.Content[i].Value] {
				return nil, "a service lists it in mapping form, which cannot name a variable"
			}
		}
		return nil, ""
	}
	if nets.Kind != yaml.SequenceNode {
		return nil, ""
	}
	var edits []lineEdit
	projectRefs := 0
	for _, n := range nets.Content {
		if n.Kind != yaml.ScalarNode {
			continue
		}
		if !keys[n.Value] {
			if projectNetworkRE.MatchString(n.Value) {
				projectRefs++
			}
			continue
		}
		old, repl := n.Value, projectNetworkName
		switch {
		case n.Style&yaml.DoubleQuotedStyle != 0:
			old, repl = `"`+old+`"`, `"`+repl+`"`
		case n.Style&yaml.SingleQuotedStyle != 0:
			old, repl = "'"+old+"'", "'"+repl+"'"
		case nets.Style&yaml.FlowStyle != 0:
			repl = `"` + repl + `"` // `{` is a flow indicator: quote the value
		}
		col, ok := byteColumn(lines, n.Line, n.Column)
		if !ok || !strings.HasPrefix(lines[n.Line-1][col-1:], old) {
			return nil, "a reference could not be located on its line"
		}
		edits = append(edits, lineEdit{line: n.Line, col: col, old: old, new: repl})
		projectRefs++
	}
	if len(edits) > 0 && projectRefs > len(edits) {
		return nil, "a service already lists the project network"
	}
	return edits, ""
}

// dropBlock plans deleting a `key: value` entry: its key line, its child
// lines and comment lines indented under it. A flow value must close on the
// key's own line.
func dropBlock(key, val *yaml.Node, lines []string) (lineEdit, bool) {
	if key.Line < 1 || key.Line > len(lines) {
		return lineEdit{}, false
	}
	if val.Style&yaml.FlowStyle != 0 {
		col, ok := byteColumn(lines, val.Line, val.Column)
		if !ok || val.Line != key.Line || !flowClosesOnLine(lines[key.Line-1][col-1:]) {
			return lineEdit{}, false
		}
		return lineEdit{line: key.Line, drop: 1}, true
	}
	indent := key.Column - 1
	end := key.Line // 0-based index of the first line after the key line
	for end < len(lines) {
		t := strings.TrimSpace(lines[end])
		if t == "" { // a blank line belongs to the block only if deeper content follows
			j := end + 1
			for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
				j++
			}
			if j >= len(lines) || leadingSpaces(lines[j]) <= indent {
				break
			}
			end = j
			continue
		}
		if leadingSpaces(lines[end]) <= indent {
			break
		}
		end++
	}
	return lineEdit{line: key.Line, drop: end - key.Line + 1}, true
}

// leadingSpaces counts the leading blanks of a line.
func leadingSpaces(l string) int { return len(l) - len(strings.TrimLeft(l, " \t")) }

// byteColumn converts a yaml.v3 column (1-based, counted in runes) on a
// 1-based line into a 1-based byte column; false when it is out of range.
func byteColumn(lines []string, line, col int) (int, bool) {
	if line < 1 || line > len(lines) || col < 1 {
		return 0, false
	}
	l, off := lines[line-1], 0
	for i := 1; i < col; i++ {
		if off >= len(l) {
			return 0, false
		}
		_, size := utf8.DecodeRuneInString(l[off:])
		off += size
	}
	return off + 1, off < len(l)
}

// flowClosesOnLine reports whether the flow collection starting at s[0]
// (a '{' or '[') is closed before the line ends. Brackets inside quoted
// strings and after a " #" comment marker do not count.
func flowClosesOnLine(s string) bool {
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#' && i > 0 && (s[i-1] == ' ' || s[i-1] == '\t'):
			return false
		case c == '{' || c == '[':
			depth++
		case c == '}' || c == ']':
			depth--
			if depth == 0 {
				return true
			}
		}
	}
	return false
}
