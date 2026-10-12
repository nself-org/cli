package database

import (
	"strings"
	"testing"
)

func TestAdditivePlanNeverDrops(t *testing.T) {
	table := map[string]any{"table": map[string]any{"schema": "public", "name": "notes"}, "select_permissions": []any{map[string]any{"role": "user", "permission": map[string]any{"columns": []any{"id"}, "filter": map[string]any{}}}}}
	live := map[string]hasuraTable{"auth.users": {Table: hasuraTableRef{Schema: "auth", Name: "users"}}}
	plan, err := additivePlan([]map[string]any{table}, live)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.BulkOps) != 2 {
		t.Fatalf("operations: %+v", plan.BulkOps)
	}
	for _, op := range plan.BulkOps {
		kind := op["type"].(string)
		if strings.Contains(kind, "drop") || strings.Contains(kind, "untrack") || kind == "replace_metadata" {
			t.Fatal(kind)
		}
	}
	live["public.notes"] = hasuraTable{Table: hasuraTableRef{Schema: "public", Name: "notes"}, SelectPermissions: []hasuraPermission{{Role: "user", Permission: map[string]any{"columns": []any{"other"}}}}}
	if _, err = additivePlan([]map[string]any{table}, live); err == nil {
		t.Fatal("permission conflict allowed")
	}
}
