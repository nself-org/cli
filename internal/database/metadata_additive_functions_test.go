package database

import (
	"reflect"
	"testing"
)

func TestAdditivePlanTracksFunction(t *testing.T) {
	fn := map[string]any{"function": map[string]any{"schema": "public", "name": "search"}, "configuration": map[string]any{"exposed_as": "query", "session_argument": "session"}, "permissions": []any{map[string]any{"role": "user"}}}
	plan, err := additivePlan([]map[string]any{fn}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.BulkOps) != 2 {
		t.Fatalf("operations: %+v", plan.BulkOps)
	}
	if plan.BulkOps[0]["type"] != "pg_track_function" || plan.BulkOps[1]["type"] != "pg_create_function_permission" {
		t.Fatal(plan.BulkOps)
	}
	args := plan.BulkOps[0]["args"].(map[string]any)
	if !reflect.DeepEqual(args["configuration"], fn["configuration"]) || !reflect.DeepEqual(args["function"], fn["function"]) {
		t.Fatal(args)
	}
	if plan.BulkOps[1]["args"].(map[string]any)["role"] != "user" {
		t.Fatal(plan.BulkOps)
	}
	for _, tc := range []struct {
		name     string
		live     map[string]any
		count    int
		conflict bool
	}{
		{"idempotent", fn, 0, false},
		{"missing permission", map[string]any{"function": fn["function"], "configuration": fn["configuration"]}, 1, false},
		{"configuration conflict", map[string]any{"function": fn["function"]}, 0, true},
		{"extra grant", map[string]any{"function": fn["function"], "configuration": fn["configuration"], "permissions": []any{map[string]any{"role": "other"}}}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := additivePlan([]map[string]any{fn}, nil, []map[string]any{tc.live})
			if (err != nil) != tc.conflict || len(plan.BulkOps) != tc.count {
				t.Fatalf("plan %+v, error %v", plan, err)
			}
		})
	}
}
