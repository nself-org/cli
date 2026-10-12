package build

// Purpose: the safety net behind TemplateSecrets: report every secret still
//          literal in the generated compose YAML, as raw bytes, inside a
//          YAML-decoded value (the encoder escapes quotes and backslashes),
//          or as a secret-named key whose value is not its bare reference.
// Inputs:  generated compose YAML, build.SecretEnvMap.
// Outputs: sorted secret names; nself build warns for each (D-0635).
// SPORT: cli/internal/build — secret templating (S-secret-env-templating).

import (
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// LiteralSecretLeaks returns the names of secrets whose literal value (>= 8
// chars) still appears in the compose YAML after templating, as raw bytes or
// inside a YAML-decoded string value (an escaped secret hides from a byte
// search). Text inside ${...} references is not a leak. Callers surface each as a build warning — the safety net behind
// TemplateSecrets.
func LiteralSecretLeaks(composeYAML []byte, secrets map[string]string) []string {
	doc := composeRefPattern.ReplaceAllString(string(composeYAML), "")
	values, byKey := decodedScalarValues(composeYAML)
	var leaked []string
	for key, val := range secrets {
		// A secret-named key whose value is not exactly its reference kept
		// part of the literal (a multi-line value Phase A cut short).
		if notRefOnly(byKey[key], key) {
			leaked = append(leaked, key)
			continue
		}
		if len(val) < 8 {
			continue
		}
		if strings.Contains(doc, val) || containsAny(values, val) {
			leaked = append(leaked, key)
		}
	}
	sort.Strings(leaked)
	return leaked
}

// decodedScalarValues returns every scalar value (mapping keys excluded) of
// the YAML document with its ${...} references removed, plus the raw values
// of the scalar mapping entries by key. Both are nil when the document does
// not parse.
func decodedScalarValues(doc []byte) ([]string, map[string][]string) {
	var root yaml.Node
	if yaml.Unmarshal(doc, &root) != nil {
		return nil, nil
	}
	var out []string
	byKey := map[string][]string{}
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		switch n.Kind {
		case yaml.ScalarNode:
			out = append(out, composeRefPattern.ReplaceAllString(n.Value, ""))
		case yaml.MappingNode:
			for i := 1; i < len(n.Content); i += 2 {
				if v := n.Content[i]; v.Kind == yaml.ScalarNode {
					k := n.Content[i-1].Value
					byKey[k] = append(byKey[k], v.Value)
				}
				walk(n.Content[i])
			}
		default:
			for _, c := range n.Content {
				walk(c)
			}
		}
	}
	walk(&root)
	return out, byKey
}

// containsAny reports whether any of values contains sub.
func containsAny(values []string, sub string) bool {
	for _, v := range values {
		if strings.Contains(v, sub) {
			return true
		}
	}
	return false
}

// notRefOnly reports whether any of values is not exactly ${key}.
func notRefOnly(values []string, key string) bool {
	for _, v := range values {
		if v != "${"+key+"}" {
			return true
		}
	}
	return false
}
