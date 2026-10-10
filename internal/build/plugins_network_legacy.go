package build

// Purpose: treats the legacy literal external network `nself_network` as the
// project network and, in v1.5, points its definition at ${DOCKER_NETWORK}.
// Inputs: the fragment bytes and their parsed top-level mapping.
// Outputs: line edits for normalizeComposeNetworks to apply.
// Constraints: byte-splice edits only (comments, quoting and layout survive).
// A def with a variable-based name is left alone. A flow-style def that does
// not close on its own line is left alone (no line-local splice is safe).
// v1.4 rewrites nothing; foreignExternalNetworks already stops reporting the
// literal in both modes.

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/compat"
)

// legacyProjectNetwork is the literal external network name older fragments
// (access-controls and others) use for the project network.
const legacyProjectNetwork = "nself_network"

// projectNetworkName is the value v1.5 writes as the network's name.
const projectNetworkName = "${DOCKER_NETWORK}"

// legacyNetworkEdits plans the v1.5 rewrite of top-level external networks
// that are the legacy literal: one with no name gets `name: ${DOCKER_NETWORK}`,
// one whose name is the literal gets that name replaced.
func legacyNetworkEdits(content []byte, top *yaml.Node, cr string) []lineEdit {
	// compat.V15(P7-PLUG-17): external network nself_network keeps its literal name -> its definition is named ${DOCKER_NETWORK}
	if !compat.V15() {
		return nil
	}
	nets := yamlGet(top, "networks")
	if nets == nil || nets.Kind != yaml.MappingNode {
		return nil
	}
	lines := strings.Split(string(content), "\n")
	var edits []lineEdit
	for i := 0; i+1 < len(nets.Content); i += 2 {
		def := nets.Content[i+1]
		ext := yamlGet(def, "external")
		if ext == nil || ext.Value == "false" {
			continue
		}
		nameNode, parent := yamlGet(def, "name"), def
		if nameNode == nil {
			nameNode, parent = yamlGet(ext, "name"), ext
		}
		switch {
		case nameNode == nil && nets.Content[i].Value == legacyProjectNetwork:
			edits = append(edits, addNetworkName(def, lines, cr)...)
		case nameNode != nil && nameNode.Kind == yaml.ScalarNode && nameNode.Value == legacyProjectNetwork:
			edits = append(edits, renameNetwork(nameNode, parent))
		}
	}
	return edits
}

// addNetworkName plans adding `name: ${DOCKER_NETWORK}` to a network def.
func addNetworkName(def *yaml.Node, lines []string, cr string) []lineEdit {
	if def.Kind != yaml.MappingNode || len(def.Content) == 0 {
		return nil
	}
	if def.Style&yaml.FlowStyle == 0 {
		first := def.Content[0]
		ind := strings.Repeat(" ", first.Column-1)
		return []lineEdit{{line: first.Line, new: ind + "name: " + projectNetworkName + cr + "\n"}}
	}
	// Flow style: add the pair before the closing brace on the def's own line.
	if def.Line < 1 || def.Line > len(lines) || def.Column < 1 || def.Column > len(lines[def.Line-1]) {
		return nil
	}
	text := lines[def.Line-1]
	depth := 0
	for j := def.Column - 1; j < len(text); j++ {
		switch text[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return []lineEdit{{line: def.Line, col: j + 1, old: "}", new: `, name: "` + projectNetworkName + `"}`}}
			}
		}
	}
	return nil
}

// renameNetwork plans replacing a literal `nself_network` name value.
func renameNetwork(name *yaml.Node, parent *yaml.Node) lineEdit {
	old, repl := name.Value, projectNetworkName
	switch {
	case name.Style&yaml.DoubleQuotedStyle != 0:
		old, repl = `"`+old+`"`, `"`+repl+`"`
	case name.Style&yaml.SingleQuotedStyle != 0:
		old, repl = "'"+old+"'", `"`+repl+`"`
	case parent.Style&yaml.FlowStyle != 0:
		repl = `"` + repl + `"` // `{` is a flow indicator: the value must be quoted
	}
	return lineEdit{line: name.Line, col: name.Column, old: old, new: repl}
}
