package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot, "schemas", "public-facts.v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("urn:nself:cli:schema:public-facts:v1", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("urn:nself:cli:schema:public-facts:v1")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func decodeJSON(t *testing.T, b []byte) any {
	t.Helper()
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// TestFactsValidateAgainstSchema checks the committed document and a fresh
// document without openapi against the generated schema, and a tampered one
// fails.
func TestFactsValidateAgainstSchema(t *testing.T) {
	s := compileSchema(t)
	committed, err := os.ReadFile(filepath.Join(repoRoot, factsPath))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(decodeJSON(t, committed)); err != nil {
		t.Fatalf("committed facts.json invalid: %v", err)
	}
	f, _ := Build([]byte("1.5.0"), []byte(testRegistry), nil)
	b, _ := Marshal(f)
	if err := s.Validate(decodeJSON(t, b)); err != nil {
		t.Fatalf("openapi-less document invalid: %v", err)
	}
	bad := bytes.Replace(b, []byte(`"version": "1.5.0"`), []byte(`"version": "v1.5"`), 1)
	if s.Validate(decodeJSON(t, bad)) == nil {
		t.Fatal("bad version must fail the schema")
	}
}
