package invoke

// Purpose: unit proof that a flag default which JSON cannot encode never
// reaches a tool schema (P7-SURF-21).
// Inputs: float flags whose default parses as NaN or an infinity.
// Outputs: test failures. Constraints: tests the guard directly, because a
// removed guard makes the MCP generator skip the tool and an end-to-end test
// could not tell the two apart.

import (
	"encoding/json"
	"testing"

	"github.com/nself-org/cli/internal/cmdregistry"
)

func TestFlagSchemaNonFiniteDefault(t *testing.T) {
	for _, d := range []string{"NaN", "nan", "+Inf", "-Inf", "Infinity", "-Infinity"} {
		s := flagSchema(cmdregistry.Flag{Name: "ratio", Type: "float64", Default: d})
		if _, err := json.Marshal(s); err != nil {
			t.Errorf("default %q: the schema cannot be encoded: %v", d, err)
		}
		if _, kept := s["default"]; kept {
			t.Errorf("default %q was kept in the schema", d)
		}
	}
	s := flagSchema(cmdregistry.Flag{Name: "ratio", Type: "float64", Default: "0.5"})
	if s["default"] != 0.5 {
		t.Errorf("a finite default was lost: %v", s["default"])
	}
}
