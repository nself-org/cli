package vectorsearch

import (
	"regexp"
	"strings"
	"testing"
)

func docsSpec(d Distance) Spec {
	return Spec{Schema: "public", Table: "docs", Column: "embedding", Dim: 3, Distance: d, Roles: []string{"user"}}
}

func TestRenderGolden(t *testing.T) {
	got, err := Render(docsSpec(Cosine))
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "docs_embedding_cosine.sql", []byte(got))
}

// TestRenderStaysInlinable pins the properties that let Postgres inline the
// function so Hasura's limit and the select filter apply to the ordered query.
func TestRenderStaysInlinable(t *testing.T) {
	for _, d := range []Distance{Cosine, L2, IP} {
		got, err := Render(docsSpec(d))
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{"LIMIT", "SECURITY DEFINER", "VOLATILE", "IMMUTABLE", "plpgsql", "SET search_path"} {
			if strings.Contains(strings.ToUpper(got), strings.ToUpper(banned)) {
				t.Errorf("%s: SQL contains %q:\n%s", d, banned, got)
			}
		}
		for _, need := range []string{"LANGUAGE sql", "STABLE", `RETURNS SETOF "public"."docs"`, "query vector(3)", `ORDER BY "embedding" ` + docsSpec(d).Operator() + " $1"} {
			if !strings.Contains(got, need) {
				t.Errorf("%s: SQL lacks %q:\n%s", d, need, got)
			}
		}
		// Only the two session timeouts may precede the function.
		for _, line := range strings.Split(got, "\n") {
			if strings.HasPrefix(line, "SET ") && !regexp.MustCompile(`^SET (lock|statement)_timeout = '[0-9a-z]+';$`).MatchString(line) {
				t.Errorf("unexpected SET line %q", line)
			}
		}
		if strings.Count(got, "SELECT") != 1 {
			t.Errorf("%s: want one SELECT:\n%s", d, got)
		}
	}
}

// TestRenderQueryNamedColumn: a column called query must not shadow the
// argument, so the body never names the parameter.
func TestRenderQueryNamedColumn(t *testing.T) {
	s := docsSpec(Cosine)
	s.Column = "query"
	got, err := Render(s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `ORDER BY "query" <=> $1`) {
		t.Errorf("body does not use $1:\n%s", got)
	}
}

func TestRenderRejectsBadInput(t *testing.T) {
	s := docsSpec(Cosine)
	s.Dim = 0
	if _, err := Render(s); err == nil {
		t.Error("Render accepted an unknown dimension")
	}
	s = docsSpec(Cosine)
	s.Table = `docs"; DROP TABLE x; --`
	if _, err := Render(s); err == nil {
		t.Error("Render accepted an injected table name")
	}
}
