package cascade

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// sample is a document that exercises every field and every enum family.
func sample() CompatV1 {
	return CompatV1{
		Schema:       SchemaV1,
		ClawContract: "claw/v1",
		NselfVersion: "1.5.0",
		GeneratedAt:  "2026-11-09T23:00:00Z",
		Entries: []MatrixEntry{
			{
				Engine:        EngineCascade,
				EngineVersion: "0.9.0",
				Status:        EntryPartial,
				VerifiedAt:    "2026-11-09T22:00:00Z",
				Notes:         "placeholder entry",
				Capabilities: []Capability{
					{Name: "tools.invoke", Status: CapSupported},
					{Name: "approvals.cards", Status: CapPartial, Notes: "text only"},
					{Name: "channels.route", Status: CapPending, Notes: "expected next minor"},
				},
			},
			{
				Engine:        EngineCascadeApp,
				EngineVersion: "0.1.0",
				Status:        EntryUnknown,
				Capabilities:  []Capability{{Name: "tools.invoke", Status: CapUnsupported}},
			},
		},
	}
}

// compile loads compat.schema.json; Compile also checks it against the
// draft-07 meta-schema, so a malformed schema fails here.
func compile(t *testing.T) (*jsonschema.Schema, map[string]any) {
	t.Helper()
	raw, err := os.ReadFile("compat.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["$schema"] != "http://json-schema.org/draft-07/schema#" {
		t.Fatalf("$schema = %v, want draft-07", doc["$schema"])
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("compat.json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("compat.json")
	if err != nil {
		t.Fatalf("schema does not compile against draft-07: %v", err)
	}
	return s, doc
}

func asAny(t *testing.T, raw []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// TestCompatV1RoundTrip: the sample validates against the schema, survives
// marshal then unmarshal unchanged, and the struct json tags equal the schema
// properties. It fails while compat.go or compat.schema.json is missing.
func TestCompatV1RoundTrip(t *testing.T) {
	s, _ := compile(t)
	raw, err := json.Marshal(sample())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(asAny(t, raw)); err != nil {
		t.Fatalf("sample does not validate: %v", err)
	}
	var back CompatV1
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, sample()) {
		t.Fatalf("round trip changed the document:\n got %+v\nwant %+v", back, sample())
	}
	checkTagsMatchSchema(t)
}

// TestCompatSchemaRejects: positive control that the schema is not vacuous.
func TestCompatSchemaRejects(t *testing.T) {
	s, _ := compile(t)
	bad := map[string]string{
		"wrong schema id":    `{"schema":"cascade.compat.v2","claw_contract":"claw/v1","entries":[]}`,
		"missing entries":    `{"schema":"cascade.compat.v1","claw_contract":"claw/v1"}`,
		"bad contract":       `{"schema":"cascade.compat.v1","claw_contract":"claw","entries":[]}`,
		"unknown top field":  `{"schema":"cascade.compat.v1","claw_contract":"claw/v1","entries":[],"x":1}`,
		"unknown engine":     `{"schema":"cascade.compat.v1","claw_contract":"claw/v1","entries":[{"engine":"other","engine_version":"1.0.0","status":"verified","capabilities":[]}]}`,
		"bad capability":     `{"schema":"cascade.compat.v1","claw_contract":"claw/v1","entries":[{"engine":"cascade","engine_version":"1.0.0","status":"verified","capabilities":[{"name":"Bad Name","status":"supported"}]}]}`,
		"bad engine version": `{"schema":"cascade.compat.v1","claw_contract":"claw/v1","entries":[{"engine":"cascade","engine_version":"v1","status":"verified","capabilities":[]}]}`,
	}
	for name, doc := range bad {
		if err := s.Validate(asAny(t, []byte(doc))); err == nil {
			t.Errorf("%s: schema accepted an invalid document", name)
		}
	}
}

// checkTagsMatchSchema: every json tag of the Go types is a property in the
// schema and every schema property has a Go field.
func checkTagsMatchSchema(t *testing.T) {
	t.Helper()
	_, doc := compile(t)
	defs := doc["definitions"].(map[string]any)
	props := func(m map[string]any) map[string]bool {
		out := map[string]bool{}
		for k := range m["properties"].(map[string]any) {
			out[k] = true
		}
		return out
	}
	cases := []struct {
		typ    reflect.Type
		schema map[string]any
	}{
		{reflect.TypeOf(CompatV1{}), doc},
		{reflect.TypeOf(MatrixEntry{}), defs["MatrixEntry"].(map[string]any)},
		{reflect.TypeOf(Capability{}), defs["Capability"].(map[string]any)},
	}
	for _, c := range cases {
		want := props(c.schema)
		got := map[string]bool{}
		for i := 0; i < c.typ.NumField(); i++ {
			tag := c.typ.Field(i).Tag.Get("json")
			name := tag
			for j := 0; j < len(tag); j++ {
				if tag[j] == ',' {
					name = tag[:j]
					break
				}
			}
			got[name] = true
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: go tags %v != schema properties %v", c.typ.Name(), got, want)
		}
	}
}
