package scratch

import (
	"strings"
	"testing"
)

func TestScratchIdentifiers(t *testing.T) {
	good := []CopyFile{{Schema: "app", Table: "notes", Columns: []string{"id", "body"}}}
	sql, schemas, err := copySQL(good)
	if err != nil || len(schemas) != 1 || !strings.Contains(sql, `"app"."notes"`) {
		t.Fatalf("copy: %s %v", sql, err)
	}
	hash, err := hashQueries(good)
	if err != nil || !strings.Contains(hash, "row_security") || !strings.Contains(hash, "bytea_output") {
		t.Fatal("hash settings missing")
	}
	for _, bad := range []CopyFile{{Schema: "public;drop", Table: "notes"}, {Schema: "app", Table: "notes;drop"}, {Schema: "app", Table: "notes", Columns: []string{"id;drop"}}} {
		if _, _, err := copySQL([]CopyFile{bad}); err == nil {
			t.Fatal("unsafe COPY identifier accepted")
		}
	}
	if _, _, err := copySQL(append(good, good...)); err == nil {
		t.Fatal("duplicate accepted")
	}
}
