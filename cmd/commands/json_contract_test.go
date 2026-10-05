package commands

// Golden contract tests for the generated JSON Schemas (P7-REG-08).
//
// Purpose: every envelope command (a key of jsonDataTypes) must ship a
// generated data schema under schemas/commands/ and a golden fixture under
// testdata/json/. The fixture must validate against the envelope schema and
// the command's data schema, and must round-trip through the Go type with
// unknown fields disallowed. .github/command-registry.json must validate
// against the registry schema.
//
// Constraints: reads only committed files; nothing here runs a command body.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/output"
)

const contractSchemas = "../../schemas"

// contractResolve loads schemas/<file>, resolving $ref targets by $id through
// schemas/index.json.
func contractResolve(t *testing.T, file string) *jsonschema.Resolved {
	t.Helper()
	read := func(f string) *jsonschema.Schema {
		b, err := os.ReadFile(filepath.Join(contractSchemas, f))
		if err != nil {
			t.Fatalf("read schema %s: %v (run `make schemas`)", f, err)
		}
		var s jsonschema.Schema
		if err := json.Unmarshal(b, &s); err != nil {
			t.Fatalf("parse schema %s: %v", f, err)
		}
		return &s
	}
	var idx struct {
		Schemas []struct{ Path, ID string }
	}
	b, err := os.ReadFile(filepath.Join(contractSchemas, "index.json"))
	if err != nil {
		t.Fatalf("read schemas/index.json: %v (run `make schemas`)", err)
	}
	if err := json.Unmarshal(b, &idx); err != nil {
		t.Fatal(err)
	}
	loader := func(u *url.URL) (*jsonschema.Schema, error) {
		for _, e := range idx.Schemas {
			if e.ID == u.String() {
				return read(e.Path), nil
			}
		}
		return nil, fmt.Errorf("no schema with id %s", u)
	}
	rs, err := read(file).Resolve(&jsonschema.ResolveOptions{Loader: loader})
	if err != nil {
		t.Fatalf("resolve %s: %v", file, err)
	}
	return rs
}

// contractValidate validates one JSON document against rs.
func contractValidate(rs *jsonschema.Resolved, doc []byte) error {
	var v any
	if err := json.Unmarshal(doc, &v); err != nil {
		return err
	}
	return rs.Validate(v)
}

func schemaFileFor(path string) string {
	return "commands/" + strings.ReplaceAll(path, " ", "-") + ".v1.schema.json"
}

func fixtureFileFor(path string) string {
	return filepath.Join("testdata", "json", strings.ReplaceAll(path, " ", "-")+".golden.json")
}

func TestJSONContract(t *testing.T) {
	if len(jsonDataTypes) == 0 {
		t.Fatal("no envelope commands registered")
	}
	envelope := contractResolve(t, "envelope.v1.schema.json")
	for path, zero := range jsonDataTypes {
		path, zero := path, zero
		t.Run(path, func(t *testing.T) {
			schemaFile := schemaFileFor(path)
			if _, err := os.Stat(filepath.Join(contractSchemas, schemaFile)); err != nil {
				t.Fatalf("envelope command %q has no schema %s: add its type to tools/schemagen and run `make schemas`", path, schemaFile)
			}
			fixture, err := os.ReadFile(fixtureFileFor(path))
			if err != nil {
				t.Fatalf("envelope command %q has no golden fixture: %v", path, err)
			}
			if err := contractValidate(envelope, fixture); err != nil {
				t.Fatalf("fixture does not validate against the envelope schema: %v", err)
			}
			var doc struct {
				SchemaVersion string          `json:"schema_version"`
				Command       string          `json:"command"`
				Data          json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(fixture, &doc); err != nil {
				t.Fatal(err)
			}
			if doc.Command != path {
				t.Fatalf("fixture command = %q, want %q", doc.Command, path)
			}
			if err := contractValidate(contractResolve(t, schemaFile), doc.Data); err != nil {
				t.Fatalf("fixture data does not validate against %s: %v", schemaFile, err)
			}
			// Round trip through the Go type, rejecting unknown fields.
			typed := reflect.New(reflect.TypeOf(zero))
			dec := json.NewDecoder(bytes.NewReader(doc.Data))
			dec.DisallowUnknownFields()
			if err := dec.Decode(typed.Interface()); err != nil {
				t.Fatalf("fixture data does not decode into %T: %v", zero, err)
			}
			var out bytes.Buffer
			w := output.Writer{Out: &out, Err: &bytes.Buffer{}}
			if err := output.EmitData(w, doc.Command, typed.Interface()); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out.Bytes(), fixture) {
				t.Fatalf("fixture does not round-trip through %T\n--- re-encoded ---\n%s\n--- fixture ---\n%s", zero, out.Bytes(), fixture)
			}
		})
	}
}

// TestJSONContractRegistryFile: the committed registry validates against the
// registry schema, and a corrupted copy does not.
func TestJSONContractRegistryFile(t *testing.T) {
	rs := contractResolve(t, "command-registry.v1.schema.json")
	b, err := os.ReadFile(committedRegistry)
	if err != nil {
		t.Fatal(err)
	}
	if err := contractValidate(rs, b); err != nil {
		t.Fatalf(".github/command-registry.json does not validate: %v", err)
	}
	bad := bytes.Replace(b, []byte(`"side_effect": "read"`), []byte(`"side_effect": "bogus"`), 1)
	if bytes.Equal(bad, b) {
		t.Fatal("test setup: no side_effect to corrupt")
	}
	if err := contractValidate(rs, bad); err == nil {
		t.Fatal("registry with side_effect bogus was accepted")
	}
}

// TestJSONContractHelpFixtureMatchesRegistry: every command in the help
// fixture equals the same command in the committed registry, so the fixture
// cannot drift from the published contract.
func TestJSONContractHelpFixtureMatchesRegistry(t *testing.T) {
	var fix struct {
		Data cmdregistry.Registry `json:"data"`
	}
	fb, err := os.ReadFile(fixtureFileFor("help"))
	if err != nil {
		t.Fatal(err)
	}
	var reg cmdregistry.Registry
	rb, err := os.ReadFile(committedRegistry)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(fb, &fix); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rb, &reg); err != nil { // _generated is ignored
		t.Fatal(err)
	}
	byPath := map[string]cmdregistry.Command{}
	for _, c := range reg.Commands {
		byPath[c.Path] = c
	}
	if len(fix.Data.Commands) == 0 {
		t.Fatal("help fixture has no commands")
	}
	for _, c := range fix.Data.Commands {
		if want, ok := byPath[c.Path]; !ok || !reflect.DeepEqual(c, want) {
			t.Errorf("fixture command %q differs from .github/command-registry.json (regenerate the fixture)", c.Path)
		}
	}
}

// TestJSONContractHelpDataRejectsBadValues: the `help` data schema carries the
// same enums, const and array types as the registry schema, so a corrupted
// copy of the golden fixture's data is rejected by the command's own schema.
func TestJSONContractHelpDataRejectsBadValues(t *testing.T) {
	fixture, err := os.ReadFile(fixtureFileFor("help"))
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(fixture, &env); err != nil {
		t.Fatal(err)
	}
	rs := contractResolve(t, schemaFileFor("help"))
	validate := func(data map[string]any) error {
		b, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		return contractValidate(rs, b)
	}
	if err := validate(env.Data); err != nil {
		t.Fatalf("test setup: unmodified fixture data rejected: %v", err)
	}
	firstCommand := func(data map[string]any) map[string]any {
		return data["commands"].([]any)[0].(map[string]any)
	}
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"canon zz", func(d map[string]any) { firstCommand(d)["canon"] = "zz" }},
		{"schema_version 2", func(d map[string]any) { d["schema_version"] = "2" }},
		{"verbs null", func(d map[string]any) { d["verbs"] = nil }},
		{"commands null", func(d map[string]any) { d["commands"] = nil }},
		{"aliases null", func(d map[string]any) { firstCommand(d)["aliases"] = nil }},
		{"core_missing null", func(d map[string]any) { d["counts"].(map[string]any)["core_missing"] = nil }},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			var cp map[string]any
			b, _ := json.Marshal(env.Data)
			if err := json.Unmarshal(b, &cp); err != nil {
				t.Fatal(err)
			}
			c.mutate(cp)
			if err := validate(cp); err == nil {
				t.Fatalf("help data with %s was accepted by %s", c.name, schemaFileFor("help"))
			}
		})
	}
}
