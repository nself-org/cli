package vectorsearch

import (
	"strings"
	"testing"
)

func TestParseTarget(t *testing.T) {
	s, err := ParseTarget("public.docs.embedding")
	if err != nil || s.Schema != "public" || s.Table != "docs" || s.Column != "embedding" {
		t.Fatalf("ParseTarget = %+v, %v", s, err)
	}
	for _, bad := range []string{"", "docs", "public.docs", "a.b.c.d", "public..embedding", ".a.b"} {
		if _, err := ParseTarget(bad); err == nil {
			t.Errorf("ParseTarget(%q) accepted a malformed target", bad)
		}
	}
}

func TestSpecValidate(t *testing.T) {
	long := strings.Repeat("x", 40)
	cases := []struct {
		name    string
		spec    Spec
		wantErr string
	}{
		{"defaults", Spec{Schema: "public", Table: "docs", Column: "embedding"}, ""},
		{"all distances", Spec{Schema: "s", Table: "t", Column: "c", Distance: IP, Roles: []string{"user", "staff"}}, ""},
		{"bad schema", Spec{Schema: "pub;lic", Table: "docs", Column: "e"}, "invalid schema"},
		{"quote in table", Spec{Schema: "public", Table: `do"cs`, Column: "e"}, "invalid table"},
		{"bad column", Spec{Schema: "public", Table: "docs", Column: "e e"}, "invalid column"},
		{"query column", Spec{Schema: "public", Table: "docs", Column: "Query"}, "collides"},
		{"name too long", Spec{Schema: "public", Table: long, Column: long}, "over the Postgres limit"},
		{"bad distance", Spec{Schema: "public", Table: "docs", Column: "e", Distance: "dot"}, "invalid --distance"},
		{"bad role", Spec{Schema: "public", Table: "docs", Column: "e", Roles: []string{"Admin1"}}, "invalid role"},
		{"role injection", Spec{Schema: "public", Table: "docs", Column: "e", Roles: []string{"a'b"}}, "invalid role"},
		{"admin role", Spec{Schema: "public", Table: "docs", Column: "e", Roles: []string{"admin"}}, "admin"},
		{"bad dim", Spec{Schema: "public", Table: "docs", Column: "e", Dim: 16001}, "dimension"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.spec.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestSpecDefaultsAndOperators(t *testing.T) {
	s := Spec{Schema: "public", Table: "docs", Column: "embedding", Roles: []string{"user", "user", "staff"}}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if s.Distance != Cosine || len(s.Roles) != 2 {
		t.Fatalf("defaults not applied: %+v", s)
	}
	empty := Spec{Schema: "public", Table: "docs", Column: "embedding"}
	_ = empty.Validate()
	if len(empty.Roles) != 1 || empty.Roles[0] != "user" {
		t.Fatalf("default roles = %v, want [user]", empty.Roles)
	}
	for d, op := range map[Distance]string{Cosine: "<=>", L2: "<->", IP: "<#>"} {
		if got := (Spec{Distance: d}).Operator(); got != op {
			t.Errorf("Operator(%s) = %s, want %s", d, got, op)
		}
	}
	if got := s.FunctionName(); got != "docs_nearest_embedding" {
		t.Errorf("FunctionName = %s", got)
	}
}
