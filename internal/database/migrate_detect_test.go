package database

import (
	"reflect"
	"strings"
	"testing"
)

func TestExtractCreatedObjects(t *testing.T) {
	sql := `
CREATE SCHEMA IF NOT EXISTS app;
CREATE TABLE app.tasks (id uuid PRIMARY KEY);
CREATE TABLE IF NOT EXISTS app.projects (id uuid PRIMARY KEY);
CREATE INDEX idx_tasks_project ON app.tasks (project_id);
CREATE TYPE app.task_status AS ENUM ('open', 'done');
CREATE MATERIALIZED VIEW app.task_counts AS SELECT 1;
CREATE VIEW app.active_tasks AS SELECT 1;
CREATE SEQUENCE app.task_seq;
`
	objs := ExtractCreatedObjects(sql)

	want := map[ObjectKind]int{
		ObjectTable:      2,
		ObjectIndex:      1,
		ObjectType:       1,
		ObjectMatView:    1,
		ObjectView:       1,
		ObjectSequence:   1,
		ObjectSchemaKind: 1,
	}
	got := map[ObjectKind]int{}
	for _, o := range objs {
		got[o.Kind]++
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("object kind counts = %+v, want %+v", got, want)
	}
}

func TestExtractCreatedObjects_NoFalsePositiveOnMaterializedView(t *testing.T) {
	objs := ExtractCreatedObjects("CREATE MATERIALIZED VIEW app.counts AS SELECT 1;")
	views, matViews := 0, 0
	for _, o := range objs {
		switch o.Kind {
		case ObjectView:
			views++
		case ObjectMatView:
			matViews++
		}
	}
	if matViews != 1 || views != 0 {
		t.Fatalf("matViews=%d views=%d, want matViews=1 views=0 (no double count)", matViews, views)
	}
}

func TestClassifyByPresence(t *testing.T) {
	tbl := ObjectRef{Kind: ObjectTable, Name: "app.tasks"}
	idx := ObjectRef{Kind: ObjectIndex, Name: "idx_tasks"}

	tests := []struct {
		name     string
		objects  []ObjectRef
		existing map[string]bool
		want     DetectClass
	}{
		{"no objects is unknown", nil, map[string]bool{}, DetectUnknown},
		{"none present is apply", []ObjectRef{tbl, idx}, map[string]bool{}, DetectApply},
		{"all present is baseline", []ObjectRef{tbl, idx}, map[string]bool{tbl.Key(): true, idx.Key(): true}, DetectBaseline},
		{"partial is conflict", []ObjectRef{tbl, idx}, map[string]bool{tbl.Key(): true}, DetectConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			class, present, missing := ClassifyByPresence(tt.objects, tt.existing)
			if class != tt.want {
				t.Fatalf("class = %s, want %s", class, tt.want)
			}
			if len(present)+len(missing) != len(tt.objects) {
				t.Fatalf("present(%d)+missing(%d) != objects(%d)", len(present), len(missing), len(tt.objects))
			}
		})
	}
}

func TestClassifyByPresence_NeverAutoResolvesConflict(t *testing.T) {
	objects := []ObjectRef{
		{Kind: ObjectTable, Name: "a"},
		{Kind: ObjectTable, Name: "b"},
	}
	existing := map[string]bool{objects[0].Key(): true} // only "a" exists
	class, present, missing := ClassifyByPresence(objects, existing)
	if class != DetectConflict {
		t.Fatalf("class = %s, want CONFLICT", class)
	}
	if len(present) != 1 || len(missing) != 1 {
		t.Fatalf("present=%d missing=%d, want 1 and 1", len(present), len(missing))
	}
}

func TestExistsExprFor(t *testing.T) {
	cases := []struct {
		o    ObjectRef
		want string
	}{
		{ObjectRef{Kind: ObjectSchemaKind, Name: "app"}, "EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'app')"},
		{ObjectRef{Kind: ObjectType, Name: "app.status"}, "to_regtype('app.status') IS NOT NULL"},
		{ObjectRef{Kind: ObjectTable, Name: "app.tasks"}, "to_regclass('app.tasks') IS NOT NULL"},
	}
	for _, c := range cases {
		if got := existsExprFor(c.o); got != c.want {
			t.Errorf("existsExprFor(%+v) = %q, want %q", c.o, got, c.want)
		}
	}
}

func TestSQLLiteral_EscapesQuotes(t *testing.T) {
	if got, want := sqlLiteral("o'brien"), "'o''brien'"; got != want {
		t.Errorf("sqlLiteral = %q, want %q", got, want)
	}
}

// TestNormalizeIdentifier is the table test for defect 2c: two spellings of
// the same object must normalize to the same string so ObjectRef.Key()
// treats them as one object, while a genuinely different schema stays
// distinct.
func TestNormalizeIdentifier(t *testing.T) {
	cases := map[string]string{
		"np_waitlist":            "np_waitlist",
		"public.np_waitlist":     "np_waitlist",
		"PUBLIC.np_waitlist":     "np_waitlist", // unquoted PUBLIC folds like Postgres does
		`"public"."np_waitlist"`: "np_waitlist",
		`"np_waitlist"`:          "np_waitlist",
		"Np_Waitlist":            "np_waitlist", // unquoted: case-folded
		`"Np_Waitlist"`:          "Np_Waitlist", // quoted: case preserved
		"app.tasks":              "app.tasks",   // non-public schema stays qualified
		`"app"."Tasks"`:          "app.Tasks",   // both segments quoted: case preserved on each
		"  np_waitlist  ":        "np_waitlist", // surrounding whitespace trimmed
	}
	for raw, want := range cases {
		if got := normalizeIdentifier(raw); got != want {
			t.Errorf("normalizeIdentifier(%q) = %q, want %q", raw, got, want)
		}
	}
}

// TestObjectRefKey_SchemaQualificationCollapses is the defect-2c regression
// at the Key() layer used throughout classifyAlterPrerequisites/
// queryExistingObjects: a table created as "np_waitlist" in one file and
// altered as "public.np_waitlist" in another must be the SAME key, or the
// alter is falsely reported as missing its prerequisite.
func TestObjectRefKey_SchemaQualificationCollapses(t *testing.T) {
	created := ObjectRef{Kind: ObjectTable, Name: "np_waitlist"}
	altered := ObjectRef{Kind: ObjectTable, Name: "public.np_waitlist"}
	if created.Key() != altered.Key() {
		t.Fatalf("Key() mismatch: created=%q altered=%q, want equal", created.Key(), altered.Key())
	}
}

// TestStripDollarQuotedBodies_BlanksDOBlockContent is defect 2a's unit: an
// ALTER TABLE written only inside a DO $$ ... $$ existence guard must not
// survive into the cleaned text at all, so ExtractAlteredObjects never sees
// it as an unconditional prerequisite.
func TestStripDollarQuotedBodies_BlanksDOBlockContent(t *testing.T) {
	sql := `DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_tables WHERE tablename = 'np_waitlist') THEN
    ALTER TABLE public.np_waitlist ADD COLUMN IF NOT EXISTS foo INT;
  END IF;
END $$;`
	cleaned := stripDollarQuotedBodies(sql)
	if strings.Contains(cleaned, "ALTER TABLE") {
		t.Fatalf("stripDollarQuotedBodies left ALTER TABLE inside the DO block: %q", cleaned)
	}
	// Newline count must be preserved so line numbers stay meaningful.
	if strings.Count(cleaned, "\n") != strings.Count(sql, "\n") {
		t.Fatalf("stripDollarQuotedBodies changed line count: got %d newlines, want %d", strings.Count(cleaned, "\n"), strings.Count(sql, "\n"))
	}
}

// TestStripDollarQuotedBodies_TaggedDelimiter covers the named-tag form
// ($body$ ... $body$), not just the bare $$ ... $$ form.
func TestStripDollarQuotedBodies_TaggedDelimiter(t *testing.T) {
	sql := `CREATE FUNCTION f() RETURNS void AS $body$
BEGIN
  ALTER TABLE widgets ADD COLUMN x INT;
END;
$body$ LANGUAGE plpgsql;`
	cleaned := stripDollarQuotedBodies(sql)
	if strings.Contains(cleaned, "ALTER TABLE widgets") {
		t.Fatalf("stripDollarQuotedBodies left content inside a tagged $body$ block: %q", cleaned)
	}
	if !strings.Contains(cleaned, "CREATE FUNCTION f()") || !strings.Contains(cleaned, "LANGUAGE plpgsql") {
		t.Fatalf("stripDollarQuotedBodies removed text outside the dollar-quoted body: %q", cleaned)
	}
}

// TestCleanSQLForObjectScan_StripsCommentsBeforeCountingDollarQuotes proves
// the fix's ordering claim: a "--"-style comment containing an unmatched "$"
// must not be mistaken for the start of a real dollar-quoted body that then
// swallows the real ALTER TABLE statement below it.
func TestCleanSQLForObjectScan_StripsCommentsBeforeCountingDollarQuotes(t *testing.T) {
	sql := "-- cost is $5 per row, see ticket\nALTER TABLE widgets ADD COLUMN x INT;"
	objs := ExtractAlteredObjects(sql)
	if len(objs) != 1 || objs[0].Name != "widgets" {
		t.Fatalf("ExtractAlteredObjects = %+v, want one table %q (comment '$' must not be read as a dollar-quote)", objs, "widgets")
	}
}
