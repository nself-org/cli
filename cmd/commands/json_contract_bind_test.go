package commands

// Command -> data binding for the JSON envelope (debt D-0243).
//
// Purpose: envelope.v1 keeps `data` open by design (it is embedded in the
// OpenAPI document and validators load only error.v1 beside it). The committed
// registry's data_schema is the published binding of each envelope command to
// its generated data schema; contractValidateBound validates a whole document
// through it, and the tests here prove the binding is total, exact and keyed on
// the document's own `command`.
//
// Constraints: reads only committed files.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/cmdregistry"
)

// contractBindings maps each envelope command path (no leading "nself ") to
// the data schema the committed registry binds it to, relative to schemas/.
// This is the published command -> data binding, the same one
// tools/jsonvalidate -data-from-registry applies.
func contractBindings(t *testing.T) map[string]string {
	t.Helper()
	b, err := os.ReadFile(committedRegistry)
	if err != nil {
		t.Fatal(err)
	}
	var reg cmdregistry.Registry
	if err := json.Unmarshal(b, &reg); err != nil { // _generated is ignored
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, c := range reg.Commands {
		if c.DataSchema != nil {
			out[strings.TrimPrefix(c.Path, "nself ")] = strings.TrimPrefix(*c.DataSchema, "schemas/")
		}
	}
	return out
}

// contractValidateBound validates one whole stdout document: the envelope
// schema, then for a success envelope the data against the schema bound to the
// document's own `command`. An unbound command is an error, so a document
// cannot name another command to dodge (or borrow) a data check. An error
// envelope is fully covered by the envelope schema (error.v1).
func contractValidateBound(t *testing.T, bindings map[string]string, doc []byte) error {
	t.Helper()
	if err := contractValidate(contractResolve(t, "envelope.v1.schema.json"), doc); err != nil {
		return fmt.Errorf("envelope schema: %w", err)
	}
	var env struct {
		Command string          `json:"command"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(doc, &env); err != nil {
		return err
	}
	if env.Data == nil {
		// No data key: the envelope schema's oneOf already required error, and
		// error.v1 checked it. "data": null is non-nil here and is validated.
		return nil
	}
	schema, ok := bindings[env.Command]
	if !ok {
		return fmt.Errorf("command %q has no data_schema in %s", env.Command, committedRegistry)
	}
	if err := contractValidate(contractResolve(t, schema), env.Data); err != nil {
		return fmt.Errorf("data does not validate against %s: %w", schema, err)
	}
	return nil
}

// TestJSONContractBindingTotal: the registry binds exactly the envelope
// commands, each to its own generated schema, so contractValidateBound checks
// every envelope command and no other.
func TestJSONContractBindingTotal(t *testing.T) {
	bindings := contractBindings(t)
	for path := range jsonDataTypes {
		if got, want := bindings[path], schemaFileFor(path); got != want {
			t.Errorf("envelope command %q: registry data_schema = %q, want %q (run `make cmd-inventory`)", path, got, want)
		}
	}
	for path := range bindings {
		if _, ok := jsonDataTypes[path]; !ok {
			t.Errorf("registry binds %q to a data schema but it is not an envelope command", path)
		}
	}
}

// TestJSONContractBindingRejectsWrongCommand: a document is checked against
// the schema of the command it names. The help fixture's data under the name
// `version`, or under an unbound name, is rejected; version data under `help`
// is rejected too.
func TestJSONContractBindingRejectsWrongCommand(t *testing.T) {
	bindings := contractBindings(t)
	rename := func(t *testing.T, fixture, to string) []byte {
		t.Helper()
		b, err := os.ReadFile(fixtureFileFor(fixture))
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal(b, &doc); err != nil {
			t.Fatal(err)
		}
		doc["command"] = to
		out, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	for _, f := range []string{"help", "version"} {
		if err := contractValidateBound(t, bindings, rename(t, f, f)); err != nil {
			t.Fatalf("test setup: unmodified %s fixture rejected: %v", f, err)
		}
	}
	cases := []struct{ fixture, command string }{
		{"help", "version"},
		{"version", "help"},
		{"help", "zz-unbound"},
	}
	for _, c := range cases {
		if err := contractValidateBound(t, bindings, rename(t, c.fixture, c.command)); err == nil {
			t.Errorf("%s data under command %q was accepted", c.fixture, c.command)
		}
	}
}
