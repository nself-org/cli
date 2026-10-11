package report

// inputs_nodes.go: the raw YAML scalar text of compose environment values.
//
// Purpose: decoding into any turns `0x1F2E...` into a decimal and a long digit
// run into a float, so the learned string differs from the text the file (and
// a log) carries. Walking yaml.Node keeps the scalar text as written (Opus S6).
// Inputs: a compose document.
// Outputs: KEY/VALUE pairs with the raw scalar text, for map and KEY=VALUE
//         list forms under environment, args and labels, at any depth.
// Constraints: aliases and merge keys are followed once each (visited set).

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// walkEnvNodes calls fn for every raw KEY/VALUE under an environment, args or
// labels key.
func walkEnvNodes(n *yaml.Node, fn func(k, v string)) {
	walkNode(n, fn, map[*yaml.Node]bool{})
}

func walkNode(n *yaml.Node, fn func(k, v string), seen map[*yaml.Node]bool) {
	if n == nil || seen[n] {
		return
	}
	seen[n] = true
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, c := range n.Content {
			walkNode(c, fn, seen)
		}
	case yaml.AliasNode:
		walkNode(n.Alias, fn, seen)
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Value == "environment" || k.Value == "args" || k.Value == "labels" {
				emitNodePairs(v, fn, map[*yaml.Node]bool{})
			}
			walkNode(v, fn, seen)
		}
	}
}

// emitNodePairs emits the pairs of one environment-like node (map or list).
func emitNodePairs(n *yaml.Node, fn func(k, v string), seen map[*yaml.Node]bool) {
	if n == nil || seen[n] {
		return
	}
	seen[n] = true
	switch n.Kind {
	case yaml.AliasNode:
		emitNodePairs(n.Alias, fn, seen)
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Value == "<<" { // merge key: the pairs of the anchor
				emitNodePairs(v, fn, seen)
				continue
			}
			for v.Kind == yaml.AliasNode && v.Alias != nil {
				v = v.Alias
			}
			if v.Kind == yaml.ScalarNode && v.Tag != "!!null" {
				fn(k.Value, v.Value)
			}
		}
	case yaml.SequenceNode:
		for _, c := range n.Content {
			for c.Kind == yaml.AliasNode && c.Alias != nil {
				c = c.Alias
			}
			if c.Kind == yaml.ScalarNode {
				if k, val, ok := strings.Cut(c.Value, "="); ok {
					fn(k, val)
				}
			}
		}
	}
}
