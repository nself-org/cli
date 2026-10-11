package database

import (
	"encoding/json"
	"strings"
	"testing"
)

// withInherited adds a top-level inherited_roles list to a liveDoc export.
func withInherited(t *testing.T, doc, inherited string) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(doc), &m); err != nil {
		t.Fatal(err)
	}
	var roles any
	if err := json.Unmarshal([]byte(inherited), &roles); err != nil {
		t.Fatal(err)
	}
	m["inherited_roles"] = roles
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestPlanTrackInheritedRoles covers D-0369: an inherited role holds select
// through its role set (nested, cycle-safe), so --roles may name it; a role
// with no select anywhere in its set is still E514.
func TestPlanTrackInheritedRoles(t *testing.T) {
	for _, tc := range []struct {
		name      string
		inherited string
		roles     []string
		wantErr   string // "" = planned
	}{
		{"direct member", `[{"role_name":"team","role_set":["user","guest"]}]`, []string{"team"}, ""},
		{"nested inherited", `[{"role_name":"org","role_set":["team"]},{"role_name":"team","role_set":["user"]}]`, []string{"org"}, ""},
		{"no member has select", `[{"role_name":"team","role_set":["guest"]}]`, []string{"team"}, "E514"},
		{"cycle without select", `[{"role_name":"a","role_set":["b"]},{"role_name":"b","role_set":["a"]}]`, []string{"a"}, "E514"},
		{"plain role without select", `[]`, []string{"guest"}, "E514"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			live := withInherited(t, liveDoc([]string{"user"}, nil, false), tc.inherited)
			ops, err := planTrack(live, "public", "docs_nearest_embedding", "docs", "embedding", tc.roles)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || ops != nil {
					t.Fatalf("want %s and no ops, got %v, %v", tc.wantErr, ops, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Join(opTypes(ops), ",")
			if got != "pg_track_function,pg_create_function_permission" {
				t.Fatalf("ops = %s", got)
			}
			if role := ops[1]["args"].(map[string]any)["role"]; role != tc.roles[0] {
				t.Errorf("permission role = %v, want %s", role, tc.roles[0])
			}
		})
	}
}

// TestPlanTrackInheritedStillChecksMembers: resolving an inherited role never
// skips the hidden-column check on the select roles of the table.
func TestPlanTrackInheritedStillChecksMembers(t *testing.T) {
	live := withInherited(t, liveDoc([]string{"user", "guest:id"}, nil, false), `[{"role_name":"team","role_set":["user"]}]`)
	if _, err := planTrack(live, "public", "docs_nearest_embedding", "docs", "embedding", []string{"team"}); err == nil ||
		!strings.Contains(err.Error(), "E514") || !strings.Contains(err.Error(), "guest") {
		t.Fatalf("want E514 naming guest, got %v", err)
	}
}

func TestVectorSelectProblem(t *testing.T) {
	for _, tc := range []struct {
		columns, query, sub string
		want                string
	}{
		{`"*"`, ``, ``, ""},
		{`["id","embedding"]`, `["select"]`, ``, ""},
		{`["id"]`, ``, ``, `hides column "embedding"`},
		{`"*"`, `[]`, ``, "query_root_fields"},
		{`"*"`, ``, `["select_by_pk"]`, "subscription_root_fields"},
	} {
		got := VectorSelectProblem(json.RawMessage(tc.columns), json.RawMessage(tc.query), json.RawMessage(tc.sub), "embedding")
		if (tc.want == "") != (got == "") || !strings.Contains(got, tc.want) {
			t.Errorf("VectorSelectProblem(%s, %s, %s) = %q, want %q", tc.columns, tc.query, tc.sub, got, tc.want)
		}
	}
}
