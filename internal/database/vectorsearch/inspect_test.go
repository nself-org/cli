package vectorsearch

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func fakeExec(out string, err error, seen *[]string) Exec {
	return func(_ context.Context, container string, cmd []string) (string, string, error) {
		if seen != nil {
			*seen = append([]string{container}, cmd...)
		}
		return out, "", err
	}
}

func TestParseDim(t *testing.T) {
	for in, want := range map[string]int{"vector(3)": 3, " vector(1536)\n": 1536, "vector(16000)": 16000} {
		if got, err := ParseDim(in); err != nil || got != want {
			t.Errorf("ParseDim(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"text", "vector", "halfvec(3)", "sparsevec(3)", "vector(0)", "vector(16001)", "vector(3);x", "integer[]", ""} {
		if _, err := ParseDim(in); err == nil {
			t.Errorf("ParseDim(%q) accepted a non-vector(n) type", in)
		}
	}
}

func TestInspectColumn(t *testing.T) {
	pg := PG{Container: "demo_postgres", User: "postgres", DB: "nself"}
	var seen []string
	dim, err := InspectColumn(t.Context(), fakeExec("vector(3)\n", nil, &seen), pg, docsSpec(Cosine))
	if err != nil || dim != 3 {
		t.Fatalf("InspectColumn = %d, %v", dim, err)
	}
	joined := strings.Join(seen, " ")
	for _, want := range []string{"demo_postgres", "psql", "-At", "-U postgres", "-d nself",
		`to_regclass('"public"."docs"')`, "a.attname = 'embedding'", "format_type(a.atttypid, a.atttypmod)"} {
		if !strings.Contains(joined, want) {
			t.Errorf("psql command lacks %q: %s", want, joined)
		}
	}
	for name, out := range map[string]string{"text column": "text\n", "missing column": "\n", "halfvec": "halfvec(3)"} {
		_, err := InspectColumn(t.Context(), fakeExec(out, nil, nil), pg, docsSpec(Cosine))
		if err == nil || !strings.Contains(err.Error(), "E513") {
			t.Errorf("%s: err = %v, want E513", name, err)
		}
	}
	if _, err := InspectColumn(t.Context(), fakeExec("", errors.New("boom"), nil), pg, docsSpec(Cosine)); err == nil || strings.Contains(err.Error(), "E513") {
		t.Errorf("exec failure must be a plain error, got %v", err)
	}
	bad := docsSpec(Cosine)
	bad.Column = "e'; DROP"
	if _, err := InspectColumn(t.Context(), fakeExec("vector(3)", nil, nil), pg, bad); err == nil {
		t.Error("InspectColumn built a query from an unvalidated column")
	}
}
