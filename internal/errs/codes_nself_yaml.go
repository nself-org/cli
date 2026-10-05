// Registry fragment: nself.yaml validation codes (E435-E436), owned by
// P7-SURF-07. Registered from init() through Register; see codes.go for the
// rules and codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E435",
			Category:   "cli",
			Summary:    "nself.yaml has a type or syntax error",
			DefaultWhy: "nself.yaml cannot be read as YAML, or a key holds a value of the wrong type (for example plugins as a number, or a plugin entry that is a map instead of a name).",
			DefaultFix: "Fix the value at the reported line so it matches schemas/nself-yaml.v1.schema.json (see the nself.yaml wiki page).",
			DocsPath:   "reference/error-codes#e435",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E436",
			Category:   "cli",
			Summary:    "Unknown key in nself.yaml",
			DefaultWhy: "nself.yaml has a key that nself does not read. Silent keys hide typos and drift.",
			DefaultFix: "Rename the key to x-<key> if it is app metadata, or remove it. nself reads app, bundle, bundles and plugins only.",
			DocsPath:   "reference/error-codes#e436",
			Exit:       1,
		},
	)
}
