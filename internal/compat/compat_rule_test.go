package compat

import (
	"encoding/json"
	"os"
	"testing"
)

// ruleRow is one row of the golden table shared with sdk/go/compat. Value nil
// means the variable is unset.
type ruleRow struct {
	Value *string `json:"value"`
	CLI   bool    `json:"cli"`
	SDK   bool    `json:"sdk"`
}

// TestRuleTableCLI asserts V15() against the cli column of
// sdk/go/compat/testdata/rule.json. The sdk module's own test reads the sdk
// column of the same file, so the plugin copy of the rule cannot drift from
// this one without a test failing.
func TestRuleTableCLI(t *testing.T) {
	raw, err := os.ReadFile("../../sdk/go/compat/testdata/rule.json")
	if err != nil {
		t.Fatalf("read rule table: %v", err)
	}
	var table struct {
		Rows []ruleRow `json:"rows"`
	}
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatalf("parse rule table: %v", err)
	}
	if len(table.Rows) == 0 {
		t.Fatal("rule table has no rows")
	}
	for _, r := range table.Rows {
		name := "unset"
		if r.Value != nil {
			name = "value=" + *r.Value
		}
		t.Run(name, func(t *testing.T) {
			if r.Value == nil {
				// t.Setenv registers the restore; unset then clears it.
				t.Setenv(EnvVar, "x")
				unset(t)
			} else {
				t.Setenv(EnvVar, *r.Value)
			}
			if got := V15(); got != r.CLI {
				t.Fatalf("V15() = %v, want %v", got, r.CLI)
			}
		})
	}
}
