package output

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

// schemasDir is the generated schema directory, relative to this package.
var schemasDir = filepath.Join("..", "..", "schemas")

// loadSchema resolves schemas/<file> with $ref targets read from the same
// directory by their urn:nself:cli:schema $id (index.json is the id map).
func loadSchema(t *testing.T, file string) *jsonschema.Resolved {
	t.Helper()
	read := func(f string) *jsonschema.Schema {
		b, err := os.ReadFile(filepath.Join(schemasDir, f))
		if err != nil {
			t.Fatalf("read %s: %v (run `make schemas`)", f, err)
		}
		var s jsonschema.Schema
		if err := json.Unmarshal(b, &s); err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		return &s
	}
	var idx struct {
		Schemas []struct{ Path, ID string }
	}
	b, err := os.ReadFile(filepath.Join(schemasDir, "index.json"))
	if err != nil {
		t.Fatalf("read index.json: %v (run `make schemas`)", err)
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

// validate checks one JSON document against rs.
func validate(rs *jsonschema.Resolved, doc string) error {
	var v any
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	return rs.Validate(v)
}

// TestEnvelopeSchemaAcceptsGoldens: every envelope golden written by the
// emitters in this package validates against envelope.v1.schema.json.
func TestEnvelopeSchemaAcceptsGoldens(t *testing.T) {
	rs := loadSchema(t, "envelope.v1.schema.json")
	files, err := filepath.Glob(filepath.Join("testdata", "*.golden.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no golden fixtures found (err=%v)", err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := validate(rs, string(b)); err != nil {
			t.Errorf("%s does not validate: %v", filepath.Base(f), err)
		}
	}
}

// TestEnvelopeSchemaRejectsNegatives: documents that break the contract fail.
func TestEnvelopeSchemaRejectsNegatives(t *testing.T) {
	rs := loadSchema(t, "envelope.v1.schema.json")
	okErr := `{"code":"E400","message":"m","exit_code":1,"class":"user"}`
	cases := map[string]string{
		"neither data nor error":       `{"schema_version":"1","command":"x"}`,
		"both data and error":          `{"schema_version":"1","command":"x","data":{},"error":` + okErr + `}`,
		"missing schema_version data":  `{"command":"x","data":{}}`,
		"missing schema_version error": `{"command":"x","error":` + okErr + `}`,
		"schema_version 2 data":        `{"schema_version":"2","command":"x","data":{}}`,
		"schema_version 2 error":       `{"schema_version":"2","command":"x","error":` + okErr + `}`,
		"schema_version number":        `{"schema_version":1,"command":"x","data":{}}`,
		"missing command":              `{"schema_version":"1","data":{}}`,
		"unknown top-level key":        `{"schema_version":"1","command":"x","data":{},"extra":1}`,
		"bad class":                    `{"schema_version":"1","command":"x","error":{"code":"E400","message":"m","exit_code":1,"class":"bogus"}}`,
		"bad code":                     `{"schema_version":"1","command":"x","error":{"code":"X1","message":"m","exit_code":1,"class":"user"}}`,
		"error missing exit_code":      `{"schema_version":"1","command":"x","error":{"code":"E400","message":"m","class":"user"}}`,
		"error not an object":          `{"schema_version":"1","command":"x","error":"boom"}`,
		"meta deprecation no removal":  `{"schema_version":"1","command":"x","data":{},"meta":{"deprecations":[{"old":"a","new":"b"}]}}`,
		"meta unknown key":             `{"schema_version":"1","command":"x","data":{},"meta":{"other":1}}`,
		"meta warnings not strings":    `{"schema_version":"1","command":"x","data":{},"meta":{"warnings":[1]}}`,
		"meta null":                    `{"schema_version":"1","command":"x","data":{},"meta":null}`,
	}
	for name, doc := range cases {
		if err := validate(rs, doc); err == nil {
			t.Errorf("%s: accepted, want rejection: %s", name, doc)
		}
	}
}

// TestEnvelopeSchemaAcceptsEveryClass: each class the error schema lists is
// accepted and an unknown class is rejected.
func TestEnvelopeSchemaAcceptsEveryClass(t *testing.T) {
	rs := loadSchema(t, "envelope.v1.schema.json")
	for _, c := range []string{"user", "infra", "auth", "destructive_blocked", "other"} {
		doc := fmt.Sprintf(`{"schema_version":"1","command":"","error":{"code":"E400","message":"m","exit_code":1,"class":%q}}`, c)
		if err := validate(rs, doc); err != nil {
			t.Errorf("class %s rejected: %v", c, err)
		}
	}
	if err := validate(rs, `{"schema_version":"1","command":"","error":{"code":"E400","message":"m","exit_code":1,"class":"nope"}}`); err == nil {
		t.Error("unknown class accepted")
	}
}
