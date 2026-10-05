package database

// Purpose: tests for the read-only default-directory planner PendingMigrations
// (P7-PROD-84): a statement-recording fake `docker` proves every statement is
// a SELECT, and an INTEGRATION=1 test proves on real Postgres that the dry run
// creates nothing and changes neither ledger.
// Inputs: temp project directories, canned ledger answers, a scratch DB.
// Outputs: assertions over recorded statements and live ledger/schema state.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pendingProject chdirs into a project with ./migrations holding files.
func pendingProject(t *testing.T, files map[string]string) {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	if err := os.MkdirAll(filepath.Join(root, "migrations"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, "migrations", name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// assertReadOnly fails unless every recorded call is a SELECT query.
func assertReadOnly(t *testing.T, stateDir string) {
	t.Helper()
	calls := recordedCalls(t, stateDir)
	if len(calls) == 0 {
		t.Fatal("no statements recorded: the fake was not used")
	}
	for _, c := range calls {
		if !strings.HasPrefix(c, "CALL QUERY SELECT ") {
			t.Errorf("non-SELECT statement in dry-run: %s", c)
		}
	}
}

func TestPendingMigrations_IssuesSelectsOnly(t *testing.T) {
	pendingProject(t, dirTestFiles)
	sd := fakeDockerState(t, map[string]string{
		"q_legacy_exists": "yes", "q_applied": "001_a.sql|2026-10-04T10:00:00Z",
	})
	got, err := PendingMigrations(context.Background(), fakeCfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "002_b.sql" {
		t.Fatalf("pending = %v, want [002_b.sql]", got)
	}
	assertReadOnly(t, sd)
}

// A database that was never migrated has no ledger table: everything is
// pending and nothing is created (no ensure*, no ledger upgrade).
func TestPendingMigrations_MissingLedgerCreatesNothing(t *testing.T) {
	pendingProject(t, dirTestFiles)
	sd := fakeDockerState(t, map[string]string{"q_legacy_exists": "no"})
	got, err := PendingMigrations(context.Background(), fakeCfg, "")
	if err != nil || len(got) != 2 {
		t.Fatalf("pending = %v, %v; want both files", got, err)
	}
	assertReadOnly(t, sd)
}

// The legacy nested-layout 'up.sql' row resolves in memory to the first nested
// migration, exactly as upgradeLedger would rename it, and still writes nothing.
func TestPendingMigrations_LegacyUpSQLRowResolvedInMemory(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	for _, d := range []string{"20260701_a", "20260702_b"} {
		p := filepath.Join(root, "hasura", "migrations", "default", d)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "up.sql"), []byte("SELECT 1;"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sd := fakeDockerState(t, map[string]string{
		"q_legacy_exists": "yes", "q_ops_exists": "yes", "q_applied": "up.sql|2026-10-04T10:00:00Z",
	})
	got, err := PendingMigrations(context.Background(), fakeCfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "20260702_b" {
		t.Fatalf("pending = %v, want [20260702_b] (nested keys are dir names)", got)
	}
	assertReadOnly(t, sd)
}

func TestPendingMigrations_MissingDirIsError(t *testing.T) {
	t.Chdir(t.TempDir())
	fakeDockerState(t, nil)
	if _, err := PendingMigrations(context.Background(), fakeCfg, ""); err == nil {
		t.Fatal("a missing migrations directory must be an error, not an empty plan")
	}
}

// A failing ledger read is an error, never an empty (all-clear) plan.
func TestPendingMigrations_LedgerErrorIsNotEmptyPlan(t *testing.T) {
	pendingProject(t, dirTestFiles)
	t.Setenv(pathEnvVar, t.TempDir()) // no docker on PATH: every query fails
	if got, err := PendingMigrations(context.Background(), fakeCfg, ""); err == nil {
		t.Fatalf("want error, got pending %v", got)
	}
}

// --- real Postgres (INTEGRATION=1) ---

func TestPendingMigrationsIntegration_DefaultDryRunWritesNothing(t *testing.T) {
	cfg := startDirPG(t)
	ctx := context.Background()
	pendingProject(t, dirTestFiles)

	relations := func() string {
		return pgScalar(t, cfg, "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')")
	}
	schemas := func() string {
		return pgScalar(t, cfg, "SELECT count(*) FROM pg_namespace WHERE nspname IN ('np_common','nself_ops')")
	}

	// 1. never-migrated database: lists both, creates no schema, no table.
	r0, s0 := relations(), schemas()
	pending, err := PendingMigrations(ctx, cfg, "")
	if err != nil || len(pending) != 2 {
		t.Fatalf("dry-run pending = %v, %v", pending, err)
	}
	if relations() != r0 || schemas() != s0 || s0 != "0" {
		t.Fatalf("dry-run changed the schema: relations %s->%s, ledger schemas %s->%s", r0, relations(), s0, schemas())
	}

	// 2. a real run applies both; the next dry-run lists none.
	if n, err := MigrateUp(ctx, cfg, ""); err != nil || n != 2 {
		t.Fatalf("up = %d, %v", n, err)
	}
	if pending, err = PendingMigrations(ctx, cfg, ""); err != nil || len(pending) != 0 {
		t.Fatalf("dry-run after up = %v, %v", pending, err)
	}

	// 3. a new pending file: the dry-run lists it and changes neither ledger.
	if err := os.WriteFile(filepath.Join("migrations", "003_c.sql"), []byte("CREATE TABLE dir_c (id int);"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger := func() string {
		return pgScalar(t, cfg, "SELECT (SELECT string_agg(name || applied_at::text, ',' ORDER BY name) FROM np_common.schema_versions) || ' / ' || (SELECT string_agg(id || checksum || applied_at::text, ',' ORDER BY id) FROM nself_ops.migrations)")
	}
	before, r1 := ledger(), relations()
	if pending, err = PendingMigrations(ctx, cfg, ""); err != nil || len(pending) != 1 || pending[0] != "003_c.sql" {
		t.Fatalf("dry-run with a new file = %v, %v", pending, err)
	}
	if after := ledger(); after != before || relations() != r1 {
		t.Fatalf("dry-run changed the database: ledger %q -> %q", before, after)
	}
	if pgScalar(t, cfg, "SELECT to_regclass('dir_c') IS NULL") != "t" {
		t.Fatal("dry-run ran the pending file")
	}
}
