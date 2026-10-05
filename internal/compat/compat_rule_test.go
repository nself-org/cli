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

// requiredRuleValues are the inputs every copy of the rule must be pinned on:
// unset (nil), the empty string, the false spellings, and every accepted
// spelling of true. A table that silently loses a row fails the test.
var requiredRuleValues = []string{"<unset>", "", "0", "1", "true", "TRUE", "True", "yes", "on", " 1"}

// requireRuleCoverage fails unless the table has a row for every required value
// (and no duplicates), so thinning rule.json cannot go unnoticed.
func requireRuleCoverage(t *testing.T, rows []ruleRow) {
	t.Helper()
	seen := map[string]int{}
	for _, r := range rows {
		key := "<unset>"
		if r.Value != nil {
			key = *r.Value
		}
		seen[key]++
	}
	for _, want := range requiredRuleValues {
		if seen[want] != 1 {
			t.Errorf("rule.json must have exactly one row for %q, has %d", want, seen[want])
		}
	}
	if len(rows) != len(requiredRuleValues) {
		t.Errorf("rule.json has %d rows, want %d (update requiredRuleValues with the rule)", len(rows), len(requiredRuleValues))
	}
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
	requireRuleCoverage(t, table.Rows)
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
