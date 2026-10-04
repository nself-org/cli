package compat

import (
	"encoding/json"
	"os"
	"testing"
)

// ruleRow is one row of testdata/rule.json. Value nil means the variable is
// unset.
type ruleRow struct {
	Value *string `json:"value"`
	CLI   bool    `json:"cli"`
	SDK   bool    `json:"sdk"`
}

// TestRuleTableSDK asserts V15() against the sdk column of the golden table the
// CLI's internal/compat test also reads (its cli column).
func TestRuleTableSDK(t *testing.T) {
	raw, err := os.ReadFile("testdata/rule.json")
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
			// t.Setenv registers the restore; Unsetenv then clears the value.
			t.Setenv(EnvVar, "x")
			if r.Value == nil {
				if err := os.Unsetenv(EnvVar); err != nil {
					t.Fatalf("unset: %v", err)
				}
			} else {
				t.Setenv(EnvVar, *r.Value)
			}
			if got := V15(); got != r.SDK {
				t.Fatalf("V15() = %v, want %v", got, r.SDK)
			}
		})
	}
}

// TestV15NotCached proves the value is re-read on every call.
func TestV15NotCached(t *testing.T) {
	t.Setenv(EnvVar, "1")
	if !V15() || Mode() != "v1.5" {
		t.Fatal("want v1.5 with NSELF_V15=1")
	}
	t.Setenv(EnvVar, "0")
	if V15() || Mode() != "v1.4" {
		t.Fatal("want v1.4 after flip to 0")
	}
}
