package lock

// keys.go — required top-level key check for Parse (P7-LIVE-09).
//
// Purpose: a hand-trimmed lock that drops a key the JSON Schema requires must
// fail to parse, rather than normalize into an empty value.
// Inputs: the raw lock bytes. Outputs: nil, or an error naming the key.
// Constraints: pure; keep requiredKeys equal to the schema's required list
// (schemas/lockfile.v1.schema.json).

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// requiredKeys are the top-level keys the JSON Schema requires and Marshal
// always writes.
var requiredKeys = []string{"schema_version", "cli", "images", "unpinned", "plugins", "bundles"}

// requireKeys fails when a required top-level key is absent or null in data.
func requireKeys(data []byte) error {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("lock: parse: %w", err)
	}
	var top *yaml.Node
	if len(doc.Content) > 0 {
		top = doc.Content[0]
	}
	have := map[string]bool{}
	if top != nil && top.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(top.Content); i += 2 {
			have[top.Content[i].Value] = top.Content[i+1].Tag != "!!null"
		}
	}
	for _, k := range requiredKeys {
		if !have[k] {
			return fmt.Errorf("lock: required key %q is missing or null", k)
		}
	}
	return nil
}
