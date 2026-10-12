package supabase

import (
	"encoding/json"
	"strings"
	"testing"
)

var cols = map[string]bool{"id": true, "user_id": true, "title": true}

func TestTranslateOwnerEquality(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    Policy
		kind []string
		col  string
		bad  string
	}{
		{"select either order", Policy{Permissive: true, Roles: []string{"authenticated"}, Cmd: "SELECT", Using: "(auth.uid() = user_id)"}, []string{"select"}, "user_id", ""},
		{"reversed", Policy{Permissive: true, Roles: []string{"public"}, Cmd: "DELETE", Using: "(user_id = auth.uid())"}, []string{"delete"}, "user_id", ""},
		{"select subquery form", Policy{Permissive: true, Roles: []string{"authenticated"}, Cmd: "UPDATE", Using: "(( SELECT auth.uid() AS uid) = user_id)", Check: "(user_id = auth.uid())"}, []string{"update"}, "user_id", ""},
		{"insert check", Policy{Permissive: true, Roles: []string{"authenticated"}, Cmd: "INSERT", Check: "(auth.uid() = user_id)"}, []string{"insert"}, "user_id", ""},
		{"all", Policy{Permissive: true, Roles: []string{"authenticated"}, Cmd: "ALL", Using: "(auth.uid() = user_id)"}, []string{"select", "insert", "update", "delete"}, "user_id", ""},
		{"restrictive", Policy{Permissive: false, Roles: []string{"public"}, Cmd: "SELECT", Using: "(auth.uid() = user_id)"}, nil, "", "restrictive"},
		{"anon role", Policy{Permissive: true, Roles: []string{"anon"}, Cmd: "SELECT", Using: "(auth.uid() = user_id)"}, nil, "", "role anon"},
		{"literal", Policy{Permissive: true, Roles: []string{"public"}, Cmd: "SELECT", Using: "true"}, nil, "", "not `auth.uid() = <column>`"},
		{"cast", Policy{Permissive: true, Roles: []string{"public"}, Cmd: "SELECT", Using: "((auth.uid())::text = user_id)"}, nil, "", "not `auth.uid() = <column>`"},
		{"and clause", Policy{Permissive: true, Roles: []string{"public"}, Cmd: "SELECT", Using: "((auth.uid() = user_id) AND (title IS NOT NULL))"}, nil, "", "not `auth.uid() = <column>`"},
		{"other column check", Policy{Permissive: true, Roles: []string{"public"}, Cmd: "UPDATE", Using: "(auth.uid() = user_id)", Check: "(auth.uid() = id)"}, nil, "", "differ"},
		{"missing column", Policy{Permissive: true, Roles: []string{"public"}, Cmd: "SELECT", Using: "(auth.uid() = owner)"}, nil, "", "not a column"},
		{"quoted column", Policy{Permissive: true, Roles: []string{"public"}, Cmd: "SELECT", Using: `(auth.uid() = "user_id")`}, []string{"select"}, "user_id", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr, bad := Translate(tc.p, cols)
			if tc.bad != "" {
				if !strings.Contains(bad, tc.bad) {
					t.Fatalf("reason %q, want %q", bad, tc.bad)
				}
				return
			}
			if bad != "" || tr.Column != tc.col || strings.Join(tr.Kinds, ",") != strings.Join(tc.kind, ",") {
				t.Fatalf("got %+v %q", tr, bad)
			}
		})
	}
}

func TestPlanListsEveryPolicy(t *testing.T) {
	res, err := Plan([]Table{{Schema: "public", Name: "notes", Columns: []string{"user_id", "id", "title"}, Policies: []Policy{
		{Schema: "public", Table: "notes", Name: "own_select", Permissive: true, Roles: []string{"authenticated"}, Cmd: "SELECT", Using: "(auth.uid() = user_id)"},
		{Schema: "public", Table: "notes", Name: "public_read", Permissive: true, Roles: []string{"public"}, Cmd: "SELECT", Using: "true"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Untranslated) != 1 || res.Untranslated[0].Table != "public.notes" || !strings.Contains(res.Untranslated[0].Policy, "public_read") || !strings.Contains(res.Untranslated[0].Policy, "USING true") {
		t.Fatalf("untranslated: %+v", res.Untranslated)
	}
	if len(res.Exemptions) != 2 || res.Tables != 1 || res.Permissions != 1 {
		t.Fatalf("counts: %+v", res)
	}
	var doc struct {
		Sources []struct {
			Tables []map[string]any `json:"tables"`
		} `json:"sources"`
	}
	if json.Unmarshal(res.Doc, &doc) != nil {
		t.Fatal("document is not JSON")
	}
	sel := doc.Sources[0].Tables[0]["select_permissions"].([]any)[0].(map[string]any)
	perm := sel["permission"].(map[string]any)
	if sel["role"] != "user" || perm["filter"].(map[string]any)["user_id"].(map[string]any)["_eq"] != "X-Hasura-User-Id" {
		t.Fatalf("permission: %v", sel)
	}
	if cs := perm["columns"].([]any); cs[0] != "id" || cs[2] != "user_id" {
		t.Fatalf("columns must be sorted: %v", cs)
	}
}

func TestPlanNoTranslatedPoliciesHasNoDocument(t *testing.T) {
	res, err := Plan([]Table{{Schema: "public", Name: "t", Columns: []string{"id"}, Policies: []Policy{{Name: "p", Permissive: true, Roles: []string{"public"}, Cmd: "ALL", Using: "true"}}}})
	if err != nil || res.Doc != nil || len(res.Untranslated) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestPlanORsPermissivePoliciesOfOneCommand(t *testing.T) {
	res, _ := Plan([]Table{{Schema: "public", Name: "t", Columns: []string{"a", "b"}, Policies: []Policy{
		{Name: "p1", Permissive: true, Roles: []string{"public"}, Cmd: "SELECT", Using: "(auth.uid() = a)"},
		{Name: "p2", Permissive: true, Roles: []string{"public"}, Cmd: "SELECT", Using: "(auth.uid() = b)"},
	}}})
	if !strings.Contains(string(res.Doc), `"_or"`) {
		t.Fatalf("two policies on one command must OR: %s", res.Doc)
	}
}

func TestPlanRestrictivePolicyBlocksTranslationOfTheTable(t *testing.T) {
	res, err := Plan([]Table{{Schema: "public", Name: "notes", Columns: []string{"id", "user_id", "archived"}, Policies: []Policy{
		{Name: "own_sel", Permissive: true, Roles: []string{"authenticated"}, Cmd: "SELECT", Using: "(auth.uid() = user_id)"},
		{Name: "no_archived", Permissive: false, Roles: []string{"public"}, Cmd: "SELECT", Using: "(NOT archived)"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Doc != nil || res.Permissions != 0 || len(res.Untranslated) != 2 {
		t.Fatalf("a restrictive policy must stop the permissive one being translated: %+v", res)
	}
	if !strings.Contains(res.Untranslated[0].Reason, "restrictive policy no_archived") {
		t.Fatalf("reason: %q", res.Untranslated[0].Reason)
	}
}

func TestPlanSkipsKindsTheRoleCannotUse(t *testing.T) {
	tbl := Table{Schema: "public", Name: "notes", Columns: []string{"id", "user_id"}, Granted: map[string]bool{"select": true}, Policies: []Policy{
		{Name: "own_all", Permissive: true, Roles: []string{"authenticated"}, Cmd: "ALL", Using: "(auth.uid() = user_id)"},
	}}
	res, _ := Plan([]Table{tbl})
	doc := string(res.Doc)
	if !strings.Contains(doc, "select_permissions") || strings.Contains(doc, "delete_permissions") || strings.Contains(doc, "insert_permissions") {
		t.Fatalf("only the granted kind may be translated: %s", doc)
	}
	tbl.Granted = map[string]bool{}
	res, _ = Plan([]Table{tbl})
	if res.Doc != nil || len(res.Untranslated) != 1 || !strings.Contains(res.Untranslated[0].Reason, "privilege") {
		t.Fatalf("no grant must list the policy: %+v", res)
	}
}

func TestTranslateNeedsUsingForReadCommands(t *testing.T) {
	_, bad := Translate(Policy{Permissive: true, Roles: []string{"public"}, Cmd: "ALL", Check: "(auth.uid() = user_id)"}, cols)
	if !strings.Contains(bad, "no USING") {
		t.Fatalf("reason %q", bad)
	}
	if _, bad = Translate(Policy{Permissive: true, Roles: []string{"public"}, Cmd: "INSERT", Check: "(auth.uid() = user_id)"}, cols); bad != "" {
		t.Fatalf("insert with only a check is fine: %q", bad)
	}
}

func TestPlanDoesNotReorderTheCallersSlice(t *testing.T) {
	in := []Table{{Schema: "z", Name: "b"}, {Schema: "a", Name: "b"}}
	_, _ = Plan(in)
	if in[0].Schema != "z" {
		t.Fatal("Plan sorted the caller's slice")
	}
}
