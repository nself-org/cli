package database

// Purpose: carry Hasura computed fields through the additive import plan.
// Inputs: bundle tables and the live tables they land on. Outputs: pg_add_computed_field ops.
// Constraints: add only; a live field with the same name and another definition is a conflict.
import (
	"encoding/json"
	"fmt"
)

type hasuraComputedField struct {
	Name       string                 `yaml:"name" json:"name"`
	Definition map[string]interface{} `yaml:"definition" json:"definition"`
	Comment    string                 `yaml:"comment,omitempty" json:"comment,omitempty"`
}

// planComputedFieldOps adds the computed fields the live table lacks. They run before permissions
// because a permission may list a computed field in its columns.
func planComputedFieldOps(key string, tableArg map[string]interface{}, lt, rt hasuraTable) ([]map[string]interface{}, []ReconcileChange) {
	var ops []map[string]interface{}
	var changes []ReconcileChange
	have := map[string]bool{}
	for _, f := range lt.ComputedFields {
		have[f.Name] = true
	}
	for _, f := range rt.ComputedFields {
		if have[f.Name] {
			continue
		}
		args := map[string]interface{}{"source": "default", "table": tableArg, "name": f.Name, "definition": f.Definition}
		if f.Comment != "" {
			args["comment"] = f.Comment
		}
		ops = append(ops, map[string]interface{}{"type": "pg_add_computed_field", "args": args})
		changes = append(changes, ReconcileChange{key, fmt.Sprintf("add computed field %q", f.Name)})
	}
	return ops, changes
}

// computedFieldConflict reports a live computed field that shares a name with an imported one
// but differs in definition or comment. The additive import never replaces it.
func computedFieldConflict(live, want hasuraTable) bool {
	for _, w := range want.ComputedFields {
		for _, g := range live.ComputedFields {
			if w.Name != g.Name {
				continue
			}
			wb, _ := json.Marshal(w)
			gb, _ := json.Marshal(g)
			if string(wb) != string(gb) {
				return true
			}
		}
	}
	return false
}
