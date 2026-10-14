package database

import (
	"strings"
	"testing"
)

func computedTable(def string) map[string]any {
	return map[string]any{
		"table": map[string]any{"schema": "public", "name": "posts"},
		"computed_fields": []any{map[string]any{
			"name":       "word_count",
			"definition": map[string]any{"function": map[string]any{"schema": "public", "name": def}, "table_argument": "post"},
			"comment":    "Post word count",
		}},
		"select_permissions": []any{map[string]any{"role": "user", "permission": map[string]any{"columns": []any{"id", "word_count"}, "filter": map[string]any{}}}},
	}
}

func TestAdditivePlanAddsComputedFieldBeforePermissions(t *testing.T) {
	plan, err := additivePlan([]map[string]any{computedTable("post_word_count")}, map[string]hasuraTable{})
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, op := range plan.BulkOps {
		kinds = append(kinds, op["type"].(string))
	}
	want := "pg_track_table,pg_add_computed_field,pg_create_select_permission"
	if got := strings.Join(kinds, ","); got != want {
		t.Fatalf("operations %s, want %s", got, want)
	}
	args := plan.BulkOps[1]["args"].(map[string]interface{})
	if args["name"] != "word_count" || args["comment"] != "Post word count" || args["definition"] == nil {
		t.Fatalf("computed field args: %+v", args)
	}
}

func TestAdditivePlanComputedFieldExistingAndConflict(t *testing.T) {
	same := hasuraTable{Table: hasuraTableRef{Schema: "public", Name: "posts"}, ComputedFields: []hasuraComputedField{{
		Name:       "word_count",
		Definition: map[string]interface{}{"function": map[string]interface{}{"schema": "public", "name": "post_word_count"}, "table_argument": "post"},
		Comment:    "Post word count",
	}}}
	plan, err := additivePlan([]map[string]any{computedTable("post_word_count")}, map[string]hasuraTable{"public.posts": same})
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range plan.BulkOps {
		if op["type"] == "pg_add_computed_field" {
			t.Fatalf("existing identical field added again: %+v", op)
		}
	}
	if _, err = additivePlan([]map[string]any{computedTable("other_function")}, map[string]hasuraTable{"public.posts": same}); err == nil || !strings.Contains(err.Error(), "computed fields") {
		t.Fatalf("definition conflict allowed: %v", err)
	}
}

func TestReconcilePlanIgnoresComputedFields(t *testing.T) {
	repo := map[string]hasuraTable{"public.posts": {Table: hasuraTableRef{Schema: "public", Name: "posts"}, ComputedFields: []hasuraComputedField{{Name: "x"}}}}
	for _, op := range buildReconcilePlanPure(repo, map[string]hasuraTable{}).BulkOps {
		if op["type"] == "pg_add_computed_field" {
			t.Fatal("non-additive reconcile changed behaviour")
		}
	}
}
