package build

// Purpose: the structural (yaml.Node) edit path of the legacy network rewrite,
// split from plugins_network_legacy.go to keep both files under the line cap.
// Inputs: the parsed fragment, its resolved view and the original bytes.
// Outputs: the re-encoded fragment, or the reason it cannot be edited.
// Constraints: only reached for shared flow entries and service aliases.

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// encodeNetworkNodes edits flow entries structurally. Service bodies are
// materialized independently so updating an alias cannot change its anchor.
// Existing legacy-reference restrictions and the resolved postcheck still apply.
func encodeNetworkNodes(doc, top *yaml.Node, res *resolvedFragment, content []byte) ([]byte, string, error) {
	keys := res.legacyKeys()
	if len(keys) > 0 {
		nets := yamlGet(top, "networks")
		if nets == nil || nets.Kind != yaml.MappingNode {
			return nil, "its definition comes from an alias or merge key", nil
		}
		kept, removed := []*yaml.Node{}, 0
		for i := 0; i+1 < len(nets.Content); i += 2 {
			if keys[nets.Content[i].Value] {
				removed++
			} else {
				kept = append(kept, nets.Content[i:i+2]...)
			}
		}
		if removed != len(keys) {
			return nil, "its definition comes from an alias or merge key", nil
		}
		nets.Content = kept
		if len(kept) == 0 {
			for i := 0; i+1 < len(top.Content); i += 2 {
				if top.Content[i+1] == nets {
					top.Content = append(top.Content[:i], top.Content[i+2:]...)
					break
				}
			}
		}
	}
	if services := yamlGet(top, "services"); services != nil && services.Kind == yaml.MappingNode {
		lines := strings.Split(string(content), "\n")
		for i := 0; i+1 < len(services.Content); i += 2 {
			name, svc := services.Content[i].Value, services.Content[i+1]
			if services.Content[i].Tag == "!!merge" {
				continue
			}
			target := svc
			if target.Kind == yaml.AliasNode {
				target = target.Alias
			}
			if _, why := legacyRefEdits(target, keys, lines); why != "" {
				return nil, why, nil
			}
			// Decode only this body, leaving extension anchors and other
			// services untouched when this service is an alias or merge.
			var body any
			if err := target.Decode(&body); err != nil {
				return nil, "", err
			}
			var materialized yaml.Node
			if err := materialized.Encode(body); err != nil {
				return nil, "", err
			}
			materialized.Anchor, materialized.Style = svc.Anchor, target.Style
			materialized.HeadComment, materialized.LineComment, materialized.FootComment = svc.HeadComment, svc.LineComment, svc.FootComment
			*svc = materialized
			if svc.Kind != yaml.MappingNode || res.Services[name].NetworkMode != nil {
				continue
			}
			nets := yamlGet(svc, "networks")
			if nets == nil && !res.Services[name].usesNetworks() {
				nets = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: projectNetworkName}}}
				svc.Content = append(svc.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "networks"}, nets)
			}
			if nets != nil {
				for j, n := range nets.Content {
					if nets.Kind == yaml.MappingNode && j%2 == 1 {
						continue
					}
					if n.Kind == yaml.ScalarNode && (keys[n.Value] || n.Value == projectNetworkName || dockerNetworkDefaultRE.MatchString(n.Value)) {
						n.Value, n.Style = projectNetworkName, yaml.DoubleQuotedStyle
					}
				}
			}
		}
	}
	out, err := yaml.Marshal(doc)
	return out, "", err
}
