package vectorsearch

import (
	"strings"
	"testing"
)

// auditDoc is an export with public.docs (select permissions perms) and the
// tracked functions fns.
func auditDoc(perms string, fns ...string) []byte {
	var f []string
	for _, n := range fns {
		schema, name, _ := strings.Cut(n, ".")
		f = append(f, `{"function":{"schema":"`+schema+`","name":"`+name+`"}}`)
	}
	return []byte(`{"version":3,"sources":[{"name":"default","tables":[` +
		`{"table":{"schema":"public","name":"docs"},"select_permissions":[` + perms + `]},` +
		`{"table":{"schema":"public","name":"notes"},"select_permissions":[{"role":"user","permission":{"columns":["id"]}}]}` +
		`],"functions":[` + strings.Join(f, ",") + `]},` +
		`{"name":"other","tables":[{"table":{"schema":"public","name":"docs"},"select_permissions":[{"role":"x","permission":{"columns":["id"]}}]}],` +
		`"functions":[{"function":{"schema":"public","name":"docs_nearest_embedding"}}]}]}`)
}

// TestAuditLive covers D-0339: a select permission added after --track that
// hides the vector column or the root field makes the tracked function unsafe.
func TestAuditLive(t *testing.T) {
	const safe = `{"role":"user","permission":{"columns":"*","filter":{}}}`
	for _, tc := range []struct {
		name     string
		live     []byte
		seen     int
		problems []string // of the one finding; nil = no finding
	}{
		{"safe", auditDoc(safe, "public.docs_nearest_embedding"), 1, nil},
		{"role added later hides the column", auditDoc(safe+`,{"role":"viewer","permission":{"columns":["id","owner"]}}`, "public.docs_nearest_embedding"),
			1, []string{`viewer: hides column "embedding"`}},
		{"role without root select", auditDoc(safe+`,{"role":"rel","permission":{"columns":"*","query_root_fields":[]}}`, "public.docs_nearest_embedding"),
			1, []string{"rel: table select is not exposed at the root (query_root_fields)"}},
		{"several roles sorted", auditDoc(`{"role":"z","permission":{"columns":["id"]}},{"role":"a","permission":{"columns":["id"]}}`, "public.docs_nearest_embedding"),
			1, []string{`a: hides column "embedding"`, `z: hides column "embedding"`}},
		{"not a search function", auditDoc(`{"role":"v","permission":{"columns":["id"]}}`, "public.search_docs", "public.docs_nearest_"), 0, nil},
		{"other schema", auditDoc(`{"role":"v","permission":{"columns":["id"]}}`, "app.docs_nearest_embedding"), 0, nil},
		{"no functions", auditDoc(`{"role":"v","permission":{"columns":["id"]}}`), 0, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, seen, err := AuditLive(tc.live, true)
			if err != nil {
				t.Fatal(err)
			}
			if seen != tc.seen {
				t.Errorf("seen = %d, want %d", seen, tc.seen)
			}
			if tc.problems == nil {
				if len(got) != 0 {
					t.Fatalf("want no finding, got %+v", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("findings = %+v, want one", got)
			}
			f := got[0]
			if f.Function != "public.docs_nearest_embedding" || f.Table != "public.docs" || f.Column != "embedding" ||
				strings.Join(f.Problems, "|") != strings.Join(tc.problems, "|") {
				t.Errorf("finding = %+v, want problems %v", f, tc.problems)
			}
		})
	}
	if _, _, err := AuditLive([]byte("{not json"), true); err == nil {
		t.Error("invalid metadata accepted")
	}
}

// TestAuditLiveAmbiguousName: a function name that fits two tables is checked
// against both, so the unsafe reading is reported.
func TestAuditLiveAmbiguousName(t *testing.T) {
	live := []byte(`{"sources":[{"name":"default","tables":[` +
		`{"table":{"schema":"public","name":"a"},"select_permissions":[{"role":"r","permission":{"columns":["id"]}}]},` +
		`{"table":{"schema":"public","name":"a_nearest_b"},"select_permissions":[{"role":"r","permission":{"columns":"*"}}]}` +
		`],"functions":[{"function":{"schema":"public","name":"a_nearest_b_nearest_c"}}]}]}`)
	got, seen, err := AuditLive(live, true)
	if err != nil || seen != 1 || len(got) != 1 || got[0].Table != "public.a" || got[0].Column != "b_nearest_c" {
		t.Fatalf("got %+v seen %d err %v", got, seen, err)
	}
}

// TestAuditLiveExplicitPermissionsOnly: with function-permission inference off
// only roles holding an explicit function permission can call the function.
func TestAuditLiveExplicitPermissionsOnly(t *testing.T) {
	live := []byte(`{"sources":[{"name":"default","tables":[{"table":{"schema":"public","name":"docs"},"select_permissions":[` +
		`{"role":"user","permission":{"columns":"*"}},{"role":"viewer","permission":{"columns":["id"]}},{"role":"staff","permission":{"columns":["id"]}}]}],` +
		`"functions":[{"function":{"schema":"public","name":"docs_nearest_embedding"},"permissions":[{"role":"user"},{"role":"staff"}]}]}]}`)
	for _, tc := range []struct {
		inferred bool
		want     string
	}{
		{true, `staff: hides column "embedding"|viewer: hides column "embedding"`},
		{false, `staff: hides column "embedding"`},
	} {
		got, seen, err := AuditLive(live, tc.inferred)
		if err != nil || seen != 1 || len(got) != 1 || strings.Join(got[0].Problems, "|") != tc.want {
			t.Errorf("inferred=%v: got %+v seen %d err %v, want %s", tc.inferred, got, seen, err, tc.want)
		}
	}
}

// TestAuditLiveInheritedCaller: with inference off, an inherited role holding
// the function permission brings in the select permissions of its role set.
func TestAuditLiveInheritedCaller(t *testing.T) {
	live := []byte(`{"inherited_roles":[{"role_name":"team","role_set":["sub"]},{"role_name":"sub","role_set":["viewer","team"]}],` +
		`"sources":[{"name":"default","tables":[{"table":{"schema":"public","name":"docs"},"select_permissions":[` +
		`{"role":"viewer","permission":{"columns":["id"]}},{"role":"other","permission":{"columns":["id"]}}]}],` +
		`"functions":[{"function":{"schema":"public","name":"docs_nearest_embedding"},"permissions":[{"role":"team"}]}]}]}`)
	got, _, err := AuditLive(live, false)
	if err != nil || len(got) != 1 || strings.Join(got[0].Problems, "|") != `viewer: hides column "embedding"` {
		t.Fatalf("got %+v err %v", got, err)
	}
}
