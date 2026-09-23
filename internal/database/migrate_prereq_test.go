package database

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func TestExtractAlteredObjects_BasicTarget(t *testing.T) {
	objs := ExtractAlteredObjects("ALTER TABLE licenses ADD COLUMN tier VARCHAR(20);")
	if len(objs) != 1 || objs[0].Kind != ObjectTable || objs[0].Name != "licenses" {
		t.Fatalf("ExtractAlteredObjects = %+v, want one table %q", objs, "licenses")
	}
}

func TestExtractAlteredObjects_SkipsGuardedIfExists(t *testing.T) {
	objs := ExtractAlteredObjects("ALTER TABLE IF EXISTS licenses ADD COLUMN tier VARCHAR(20);")
	if len(objs) != 0 {
		t.Fatalf("ExtractAlteredObjects on a guarded ALTER = %+v, want none (IF EXISTS cannot fail on a missing table)", objs)
	}
}

func TestExtractAlteredObjects_HandlesOnlyAndSchemaQualified(t *testing.T) {
	objs := ExtractAlteredObjects("ALTER TABLE ONLY app.tasks ADD COLUMN done boolean;")
	if len(objs) != 1 || objs[0].Name != "app.tasks" {
		t.Fatalf("ExtractAlteredObjects = %+v, want one table %q", objs, "app.tasks")
	}
}

// TestExtractAlteredObjects_FalsePositiveShapes is the table test for
// defect 2's three reported false-positive shapes, reproducing the exact
// refusal text observed against a prod-shaped database ("ENABLE", "ADD",
// "in" reported as missing tables needed by migration 024).
func TestExtractAlteredObjects_FalsePositiveShapes(t *testing.T) {
	cases := []struct {
		name string
		sql  string
	}{
		{
			name: "ALTER TABLE mentioned in a line comment across a line break",
			sql: "-- Later migrations will:\n" +
				"--   ALTER TABLE\n" +
				"--     ENABLE ROW LEVEL SECURITY\n" +
				"--     ADD CONSTRAINT ... in production\n",
		},
		{
			name: "ALTER TABLE mentioned in a block comment",
			sql:  "/* ALTER TABLE\n     ENABLE ROW LEVEL SECURITY */\n",
		},
		{
			name: "ALTER TABLE only inside a guarded DO block",
			sql: "DO $$\nBEGIN\n" +
				"  IF EXISTS (SELECT 1 FROM pg_tables WHERE tablename = 'np_users') THEN\n" +
				"    ALTER TABLE public.np_users ADD COLUMN IF NOT EXISTS foo INT;\n" +
				"  END IF;\nEND $$;\n",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			objs := ExtractAlteredObjects(tt.sql)
			if len(objs) != 0 {
				t.Fatalf("ExtractAlteredObjects(%q) = %+v, want none (no real unconditional ALTER TABLE present)", tt.sql, objs)
			}
		})
	}
}

// TestExtractAlteredObjects_RealAlterStillDetectedAlongsideComment guards
// against stripping being too aggressive: a genuine ALTER TABLE elsewhere in
// the same file must still be found even when the file also contains one of
// the false-positive shapes above.
func TestExtractAlteredObjects_RealAlterStillDetectedAlongsideComment(t *testing.T) {
	sql := "-- ALTER TABLE\n--   ENABLE ROW LEVEL SECURITY\n" +
		"ALTER TABLE np_waitlist ADD COLUMN IF NOT EXISTS foo INT;\n"
	objs := ExtractAlteredObjects(sql)
	if len(objs) != 1 || objs[0].Name != "np_waitlist" {
		t.Fatalf("ExtractAlteredObjects = %+v, want exactly one real target %q", objs, "np_waitlist")
	}
}

func TestClassifyAlterPrerequisites_SatisfiedByLiveSchema(t *testing.T) {
	files := []parsedMigration{
		{Name: "009_licensing_tiers.sql", Altered: []ObjectRef{{Kind: ObjectTable, Name: "licenses"}}},
	}
	existing := map[string]bool{ObjectRef{Kind: ObjectTable, Name: "licenses"}.Key(): true}

	missing := classifyAlterPrerequisites(files, existing)
	if len(missing) != 0 {
		t.Fatalf("classifyAlterPrerequisites = %+v, want none (table already exists live)", missing)
	}
}

func TestClassifyAlterPrerequisites_SatisfiedByEarlierFileInBatch(t *testing.T) {
	files := []parsedMigration{
		{Name: "001_create_widgets.sql", Created: []ObjectRef{{Kind: ObjectTable, Name: "widgets"}}},
		{Name: "002_alter_widgets.sql", Altered: []ObjectRef{{Kind: ObjectTable, Name: "widgets"}}},
	}
	missing := classifyAlterPrerequisites(files, map[string]bool{})
	if len(missing) != 0 {
		t.Fatalf("classifyAlterPrerequisites = %+v, want none (widgets is created earlier in the same batch)", missing)
	}
}

// TestClassifyAlterPrerequisites_SatisfiedByOwnFile is the regression this
// fix locks: a single migration file that both CREATEs a table and then
// ALTERs the same table (the common "CREATE ...; ALTER ... ADD CONSTRAINT
// ...;" authoring shape) must never be flagged as missing a prerequisite —
// the table it needs is created earlier in the very same file.
func TestClassifyAlterPrerequisites_SatisfiedByOwnFile(t *testing.T) {
	files := []parsedMigration{
		{
			Name:    "20260901_bad_constraint.sql",
			Created: []ObjectRef{{Kind: ObjectTable, Name: "accounts"}},
			Altered: []ObjectRef{{Kind: ObjectTable, Name: "accounts"}},
		},
	}
	missing := classifyAlterPrerequisites(files, map[string]bool{})
	if len(missing) != 0 {
		t.Fatalf("classifyAlterPrerequisites = %+v, want none (accounts is created earlier in the same file)", missing)
	}
}

func TestClassifyAlterPrerequisites_RefusesWhenNeitherLiveNorInBatch(t *testing.T) {
	files := []parsedMigration{
		{Name: "009_licensing_tiers.sql", Altered: []ObjectRef{{Kind: ObjectTable, Name: "licenses"}}},
	}
	missing := classifyAlterPrerequisites(files, map[string]bool{})
	if len(missing) != 1 {
		t.Fatalf("classifyAlterPrerequisites = %+v, want exactly one missing prerequisite", missing)
	}
	if missing[0].Object.Name != "licenses" || missing[0].MigrationID != "009_licensing_tiers.sql" {
		t.Fatalf("missing prerequisite = %+v, want licenses needed by 009_licensing_tiers.sql", missing[0])
	}
}

// TestClassifyAlterPrerequisites_LaterCreateDoesNotSatisfyEarlierAlter proves
// ordering matters: a CREATE TABLE that only appears in a LATER file must not
// satisfy an ALTER TABLE in an earlier one, since the earlier migration would
// run first and still hit a missing relation.
func TestClassifyAlterPrerequisites_LaterCreateDoesNotSatisfyEarlierAlter(t *testing.T) {
	files := []parsedMigration{
		{Name: "001_alter_widgets.sql", Altered: []ObjectRef{{Kind: ObjectTable, Name: "widgets"}}},
		{Name: "002_create_widgets.sql", Created: []ObjectRef{{Kind: ObjectTable, Name: "widgets"}}},
	}
	missing := classifyAlterPrerequisites(files, map[string]bool{})
	if len(missing) != 1 || missing[0].MigrationID != "001_alter_widgets.sql" {
		t.Fatalf("classifyAlterPrerequisites = %+v, want one missing prerequisite against 001_alter_widgets.sql", missing)
	}
}

// TestClassifyAlterPrerequisites_SchemaQualificationDoesNotFalsePositive is
// defect 2c's regression at the classification layer: a table created
// unqualified in an earlier file and altered "public."-qualified in a later
// one (or vice versa) is the SAME object and must not be refused.
func TestClassifyAlterPrerequisites_SchemaQualificationDoesNotFalsePositive(t *testing.T) {
	files := []parsedMigration{
		{Name: "001_create.sql", Created: []ObjectRef{{Kind: ObjectTable, Name: "np_waitlist"}}},
		{Name: "002_alter.sql", Altered: []ObjectRef{{Kind: ObjectTable, Name: "public.np_waitlist"}}},
	}
	missing := classifyAlterPrerequisites(files, map[string]bool{})
	if len(missing) != 0 {
		t.Fatalf("classifyAlterPrerequisites = %+v, want none (public.np_waitlist and np_waitlist are the same table)", missing)
	}

	// And the live-schema side: existing keyed unqualified must satisfy an
	// ALTER written schema-qualified.
	files2 := []parsedMigration{
		{Name: "024_alter.sql", Altered: []ObjectRef{{Kind: ObjectTable, Name: "public.np_users"}}},
	}
	existing := map[string]bool{ObjectRef{Kind: ObjectTable, Name: "np_users"}.Key(): true}
	if missing := classifyAlterPrerequisites(files2, existing); len(missing) != 0 {
		t.Fatalf("classifyAlterPrerequisites = %+v, want none (np_users exists live, public.np_users is the same table)", missing)
	}
}

// TestClassifyAlterPrerequisites_GenuineRefusalSurvivesNormalization is the
// "keep the checker's real protection" requirement: normalizing identifiers
// and stripping comments/DO-block bodies must never mask a genuine missing
// prerequisite — a table that truly does not exist live and is not created
// by any earlier (or the same) file in the batch must still be refused, even
// once schema-qualification is normalized away.
func TestClassifyAlterPrerequisites_GenuineRefusalSurvivesNormalization(t *testing.T) {
	files := []parsedMigration{
		{Name: "024_add_column.sql", Altered: []ObjectRef{{Kind: ObjectTable, Name: "public.np_users"}}},
	}
	missing := classifyAlterPrerequisites(files, map[string]bool{}) // nothing live, nothing created earlier
	if len(missing) != 1 {
		t.Fatalf("classifyAlterPrerequisites = %+v, want exactly one missing prerequisite (np_users genuinely absent)", missing)
	}
	if missing[0].Object.Name != "public.np_users" || missing[0].MigrationID != "024_add_column.sql" {
		t.Fatalf("missing prerequisite = %+v, want public.np_users needed by 024_add_column.sql", missing[0])
	}
}

func TestClassifyAlterPrerequisites_DedupesRepeatedTarget(t *testing.T) {
	files := []parsedMigration{
		{Name: "009_a.sql", Altered: []ObjectRef{{Kind: ObjectTable, Name: "licenses"}}},
		{Name: "010_b.sql", Altered: []ObjectRef{{Kind: ObjectTable, Name: "licenses"}}},
	}
	missing := classifyAlterPrerequisites(files, map[string]bool{})
	if len(missing) != 1 {
		t.Fatalf("classifyAlterPrerequisites = %+v, want a single deduped entry for the repeated target", missing)
	}
}

// TestClassifyAlterPrerequisites_EmptyBatchIsIdempotent covers the
// idempotency case at the pure-function layer: an empty pending batch (every
// migration already applied) must never report a missing prerequisite —
// exercised again end-to-end via checkAlterPrerequisites below.
func TestClassifyAlterPrerequisites_EmptyBatchIsIdempotent(t *testing.T) {
	missing := classifyAlterPrerequisites(nil, map[string]bool{})
	if len(missing) != 0 {
		t.Fatalf("classifyAlterPrerequisites(nil, ...) = %+v, want none", missing)
	}
}

// TestCheckAlterPrerequisites_SkipsEntirelyWhenNothingPending is the
// idempotency guarantee at the checkAlterPrerequisites layer: once every
// migration in a batch is applied, the caller passes an empty pending slice
// and this must return immediately without touching the database (a nil
// *config.Config would panic if it tried).
func TestCheckAlterPrerequisites_SkipsEntirelyWhenNothingPending(t *testing.T) {
	missing, err := checkAlterPrerequisites(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("checkAlterPrerequisites(nil files) error = %v, want nil", err)
	}
	if len(missing) != 0 {
		t.Fatalf("checkAlterPrerequisites(nil files) = %+v, want none", missing)
	}
}

func TestHasuraMigrationsDirIfPresent_NestedLayout(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(filepath.Join("hasura", "migrations", "default"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// hasuraMigrationsDirIfPresent returns its candidate list's literal
	// forward-slash form (matching migrationsDir's own candidates in
	// migrate_ledger.go), not a filepath.Join result — os.Stat accepts "/"
	// on Windows too, but filepath.Join there would produce "\"-separated
	// segments that never equal the literal the function actually returns.
	const want = "hasura/migrations/default"
	got, ok := hasuraMigrationsDirIfPresent()
	if !ok || got != want {
		t.Fatalf("hasuraMigrationsDirIfPresent() = (%q, %v), want (%q, true)", got, ok, want)
	}
}

func TestHasuraMigrationsDirIfPresent_AbsentReturnsFalse(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, ok := hasuraMigrationsDirIfPresent(); ok {
		t.Fatalf("hasuraMigrationsDirIfPresent() = true in an empty directory, want false")
	}
}

// TestPrerequisiteError_NamesObjectAndHasuraCommand is the refuse-first
// contract's message check: it must name the missing table, that it comes
// from Hasura's migrations, the specific Hasura migration that creates it
// when one is found on disk, and the exact remediation command.
func TestPrerequisiteError_NamesObjectAndHasuraCommand(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	hasuraMigDir := filepath.Join("hasura", "migrations", "default", "1706140802000_licenses_and_telemetry")
	if err := os.MkdirAll(hasuraMigDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hasuraMigDir, "up.sql"), []byte("CREATE TABLE licenses (id uuid PRIMARY KEY);"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	err := prerequisiteError([]MissingPrerequisite{
		{Object: ObjectRef{Kind: ObjectTable, Name: "licenses"}, MigrationID: "009_licensing_tiers.sql"},
	})
	if err == nil {
		t.Fatal("prerequisiteError(...) = nil, want a non-nil refusal")
	}
	if !errors.Is(err, errs.ErrMigrationPrerequisiteMissing) {
		t.Fatalf("prerequisiteError(...) does not wrap ErrMigrationPrerequisiteMissing: %v", err)
	}

	msg := err.Error()
	for _, want := range []string{
		`"licenses"`,
		"009_licensing_tiers.sql",
		"1706140802000_licenses_and_telemetry",
		"hasura migrate apply --database-name default",
		"nself db migrate up",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("prerequisiteError message %q does not contain %q", msg, want)
		}
	}
}

// TestPrerequisiteError_NoHasuraDirFallsBackGenerically covers a project with
// no Hasura migrations at all (e.g. ntask's postgres/migrations layout):
// the refusal must still name the object without falsely claiming Hasura
// ownership.
func TestPrerequisiteError_NoHasuraDirFallsBackGenerically(t *testing.T) {
	t.Chdir(t.TempDir())

	err := prerequisiteError([]MissingPrerequisite{
		{Object: ObjectRef{Kind: ObjectTable, Name: "widgets"}, MigrationID: "002_alter_widgets.sql"},
	})
	msg := err.Error()
	if !strings.Contains(msg, `"widgets"`) || !strings.Contains(msg, "002_alter_widgets.sql") {
		t.Fatalf("prerequisiteError message %q does not name the missing object/migration", msg)
	}
	if strings.Contains(msg, "hasura migrate apply") {
		t.Fatalf("prerequisiteError message %q claims a Hasura fix command with no Hasura directory present", msg)
	}
}

// TestParseMigrationFile_MatchesExtractHelpers is a smoke test that
// parseMigrationFile wires ExtractCreatedObjects/ExtractAlteredObjects
// together correctly rather than diverging from them.
func TestParseMigrationFile_MatchesExtractHelpers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "20260101_widgets.sql")
	sql := "CREATE TABLE widgets (id serial primary key);\nALTER TABLE gadgets ADD COLUMN done boolean;"
	if err := os.WriteFile(path, []byte(sql), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	p, err := parseMigrationFile(path)
	if err != nil {
		t.Fatalf("parseMigrationFile: %v", err)
	}
	if p.Name != "20260101_widgets.sql" {
		t.Errorf("Name = %q, want 20260101_widgets.sql", p.Name)
	}
	if len(p.Created) != 1 || p.Created[0].Name != "widgets" {
		t.Errorf("Created = %+v, want one table %q", p.Created, "widgets")
	}
	if len(p.Altered) != 1 || p.Altered[0].Name != "gadgets" {
		t.Errorf("Altered = %+v, want one table %q", p.Altered, "gadgets")
	}
}
