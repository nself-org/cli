package database

// Purpose: round-trip shapes for the metadata layout (D-0272 fix round 1): a
// one-line file is inlined onto its key line by resolveIncludes, and a string
// holding "!include" must not read as an include directive.
// Inputs: small inline JSON documents. Outputs: pass/fail.
// Constraints: no Hasura, no network.

import (
	"bytes"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestHasuraLayoutShapesRoundTrip(t *testing.T) {
	for name, in := range map[string]string{
		"one-key map":             `{"graphql_schema_introspection":{"disabled_for_roles":[]},"version":3}`,
		"map with empty list":     `{"metrics_config":{"analyze_query_variables":true,"x":[]},"version":3}`,
		"one-key bool map":        `{"metrics_config":{"analyze_query_variables":true}}`,
		"one-element scalar list": `{"allowlist":["x"],"version":3}`,
		"empty list":              `{"remote_schemas":[],"version":3}`,
		"empty map":               `{"api_limits":{},"version":3}`,
		"scalar version":          `{"version":3}`,
		"string with newline":     `{"note":"line one\nline two\n","version":3}`,
		"nested newline":          `{"actions":[{"comment":"a\nb"}],"version":3}`,
		"trailing blank lines":    `{"actions":[{"comment":"a\n\n"}],"version":3}`,
		"trailing three newlines": `{"note":"a\nb\n\n\n","version":3}`,
		"include text scalar":     `{"note":"see !include foo.yaml","version":3}`,
		"include text nested":     `{"actions":[{"comment":"see !include foo.yaml"},{"x":"!include"}],"version":3}`,
		"include text in list":    `{"allowlist":["see !include foo.yaml"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			dest := unpackLayout(t, []byte(in))
			if err := validateArchiveIncludes(dest); err != nil {
				t.Fatalf("validateArchiveIncludes: %v", err)
			}
			resolved, err := resolveIncludes(dest, filepath.Join(dest, "config.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			var doc interface{}
			if err := yaml.Unmarshal(resolved, &doc); err != nil {
				t.Fatalf("resolved config is not YAML: %v\n%s", err, resolved)
			}
			sameJSON(t, convertYAMLToJSON(doc), []byte(in))
		})
	}
}

// The files in the archive never carry an include directive other than the index lines.
func TestHasuraLayoutIncludeTextOnlyInIndex(t *testing.T) {
	files, err := metadataLayout([]byte(`{"note":"see !include foo.yaml","a":{"b":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Name != metadataIndexFile && bytes.Contains(f.Data, []byte("!include")) {
			t.Errorf("%s contains an include directive: %s", f.Name, f.Data)
		}
	}
}
