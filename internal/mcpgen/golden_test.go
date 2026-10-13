package mcpgen_test

// Purpose: the tool list golden for the hermetic tree (v1.5 mode) and the
// invariants every generated tool must hold.
// Constraints: -update rewrites testdata/tools.golden.json; review the diff.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/mcpgen"
	"github.com/nself-org/cli/schemas"
)

func noInvoke(context.Context, string, invoke.Request) (invoke.Result, error) {
	return invoke.Result{}, nil
}

func TestToolListGolden(t *testing.T) {
	reg := hermeticRegistry(t)
	tools, err := mcpgen.Tools(mcpgen.Options{Registry: reg, Invoke: noInvoke, Schema: schemas.Lookup, V15: true})
	if err != nil {
		t.Fatal(err)
	}
	list := make([]json.RawMessage, 0, len(tools))
	for _, st := range tools {
		b, err := json.Marshal(st.Tool)
		if err != nil {
			t.Fatalf("%s: %v", st.Tool.Name, err)
		}
		list = append(list, b)
	}
	golden(t, "tools.golden.json", marshalIndent(t, map[string]any{"tools": list}))
}
