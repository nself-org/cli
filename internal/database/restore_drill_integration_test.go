//go:build integration

// Package database integration tests for RestoreDrill against a REAL,
// multi-schema Postgres fixture — the exact shape that hid the production
// defect: a drill that verifies zero rows must never report success, on
// ANY caller of RestoreDrill(), not only the ones that go through Drill()'s
// smokeCheck wrapper.
//
// Background: `nself db restore-drill` (cmd/commands/db_pitr_ops.go) calls
// database.RestoreDrill() directly and trusts RestoreDrillResult.Success.
// Before this fix, RestoreDrill() set Success = true unconditionally once
// the restore + verify + critical-table steps returned no Go error — it
// never looked at RowsVerified itself; only Drill()'s separate smokeCheck
// call did. A drill run through `nself db restore-drill` on a multi-schema
// stack (tables spread beyond `public`, like nself-web: telemetry_events,
// plugin_downloads, provider_requests) could restore genuinely nothing and
// still print "Drill status: PASS". These tests reproduce that shape
// end-to-end against a real Postgres container rather than only unit-testing
// the pure smokeCheck helper.
//
// Run with:
//
//	INTEGRATION=1 go test -mod=vendor -tags integration -timeout 180s \
//	    ./internal/database/... -run TestRestoreDrill_Integration
package database

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureSchemaStmts creates a multi-schema layout — tables split across
// `public` and `app_data`, mirroring nself-web's shape where data lives
// outside the `public` schema alone. withData controls whether the tables
// are populated: the zero-row regression test needs an otherwise-identical,
// otherwise-healthy restore (same table count, same schemas) that simply has
// no rows, so the only variable under test is RowsVerified.
func fixtureSchemaStmts(withData bool) []string {
	stmts := []string{
		"CREATE SCHEMA IF NOT EXISTS app_data",
		"CREATE TABLE public.telemetry_events (id serial primary key, payload text)",
		"CREATE TABLE public.plugin_downloads (id serial primary key, name text)",
		"CREATE TABLE app_data.provider_requests (id serial primary key, note text)",
		"CREATE TABLE app_data.np_users (id serial primary key, email text)",
		"CREATE TABLE app_data.np_licenses (id serial primary key, key text)",
		"CREATE TABLE app_data.np_audit_log (id serial primary key, action text)",
		"CREATE TABLE app_data.np_plugins (id serial primary key, slug text)",
		"CREATE TABLE app_data.np_billing (id serial primary key, amount int)",
	}
	if withData {
		stmts = append(stmts,
			"INSERT INTO public.telemetry_events (payload) SELECT 'evt-'||g FROM generate_series(1,50) g",
			"INSERT INTO public.plugin_downloads (name) SELECT 'plugin-'||g FROM generate_series(1,20) g",
			"INSERT INTO app_data.provider_requests (note) SELECT 'req-'||g FROM generate_series(1,10) g",
		)
	}
	return stmts
}

// buildFixtureDump applies fixtureSchemaStmts to the running container's
// "nself" database, then pg_dumps it (custom format, matching every real
// backup produced by `nself backup create`) to a file under dir. Returns the
// dump file path RestoreDrill can consume exactly like a real backup.
func buildFixtureDump(t *testing.T, container, dir string, withData bool) string {
	t.Helper()

	for _, stmt := range fixtureSchemaStmts(withData) {
		out, err := exec.Command("docker", "exec", container,
			"psql", "-U", "postgres", "-d", "nself", "-c", stmt).CombinedOutput()
		if err != nil {
			t.Fatalf("fixture setup %q: %v\n%s", stmt, err, out)
		}
	}

	dumpPath := filepath.Join(dir, "fixture.dump")
	f, err := os.Create(dumpPath)
	if err != nil {
		t.Fatalf("create dump file: %v", err)
	}
	defer func() { _ = f.Close() }()

	cmd := exec.Command("docker", "exec", container, "pg_dump", "-U", "postgres", "-d", "nself", "-Fc")
	cmd.Stdout = f
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("pg_dump: %v: %s", err, stderr.String())
	}
	return dumpPath
}

// TestRestoreDrill_Integration_MultiSchemaRowsCounted proves the healthy
// case still works: a restore whose data spans more than one schema must be
// recognized (tables + rows both counted across every non-system schema, not
// just `public`), and RestoreDrill must report success.
func TestRestoreDrill_Integration_MultiSchemaRowsCounted(t *testing.T) {
	skipUnlessIntegration(t)
	cfg := startTestPostgres(t)
	container := containerName(cfg)

	dumpPath := buildFixtureDump(t, container, t.TempDir(), true /* withData */)

	result, err := RestoreDrill(context.Background(), cfg, dumpPath)
	if err != nil {
		t.Fatalf("RestoreDrill: unexpected error for a real multi-schema restore: %v (result: %+v)", err, result)
	}
	if !result.Success {
		t.Fatalf("RestoreDrill.Success = false for a real multi-schema restore, want true (result: %+v)", result)
	}
	if result.RowsVerified != 80 { // 50 + 20 + 10, per fixtureSchemaStmts
		t.Errorf("RestoreDrill.RowsVerified = %d, want 80 (rows must be summed across ALL schemas, not just public)", result.RowsVerified)
	}
	if result.TablesVerified < 8 {
		t.Errorf("RestoreDrill.TablesVerified = %d, want >= 8 (tables span public + app_data)", result.TablesVerified)
	}
}

// TestRestoreDrill_Integration_ZeroRowsNeverSucceeds is the regression lock
// for the production defect: a restore with real tables (well past the
// table-count floor) but zero rows in every one of them must NEVER report
// success — on RestoreDrill() itself, not only on the Drill() wrapper around
// it. This is exactly the code path `nself db restore-drill`
// (cmd/commands/db_pitr_ops.go) exercises directly.
//
// Before the fix in this commit, RestoreDrill() set Success = true
// unconditionally once restore/verify/critical-table steps returned no Go
// error, never consulting RowsVerified — this test fails against that code
// (result.Success == true, err == nil) and passes against the fix.
func TestRestoreDrill_Integration_ZeroRowsNeverSucceeds(t *testing.T) {
	skipUnlessIntegration(t)
	cfg := startTestPostgres(t)
	container := containerName(cfg)

	dumpPath := buildFixtureDump(t, container, t.TempDir(), false /* schema-only, no rows */)

	result, err := RestoreDrill(context.Background(), cfg, dumpPath)

	if result.Success {
		t.Fatalf("RestoreDrill.Success = true for a zero-row restore (tables=%d rows=%d) — a drill that verified zero rows must never report success",
			result.TablesVerified, result.RowsVerified)
	}
	if err == nil {
		t.Fatalf("RestoreDrill returned err == nil for a zero-row restore; want a non-nil error surfacing the hard fail")
	}
	if result.RowsVerified != 0 {
		t.Errorf("RestoreDrill.RowsVerified = %d, want 0 for a schema-only restore", result.RowsVerified)
	}
	if result.ErrorMessage == "" {
		t.Errorf("RestoreDrillResult.ErrorMessage is empty; want the zero-row smoke-check message recorded on the result")
	}
}
