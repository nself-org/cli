//go:build integration

// schema_reconcile_integration_test.go — live-Postgres verification for
// reconcileSchemaVersionsTable, closing the live-verification gap
// schema_container_test.go's doc comment flags for this package: nothing
// previously exercised this reconciliation logic against a REAL Postgres
// instance carrying the legacy migration-ledger table shape that broke
// "nself plugin update cron" in production (nself 1.4.8, 2026-09-23).
//
// Run with:
//
//	INTEGRATION=1 go test -tags integration -timeout 120s \
//	    ./internal/plugin/... -run TestReconcileSchemaVersionsTable_Integration
package plugin

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// skipUnlessIntegration mirrors the convention used by
// internal/database/migrate_integration_test.go: opt-in via INTEGRATION=1,
// Docker required.
func skipUnlessIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run Docker Postgres integration tests")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found in PATH")
	}
}

// integrationTestConfig points at the throwaway Postgres container these
// tests share. Unlike internal/database's per-test containers, every
// subtest here resets np_common itself (see resetSchemaVersionsTable), so
// one shared container for the whole run is enough. Override the project
// name via NSELF_TEST_POSTGRES_PROJECT if the default container name is
// already taken on the host running the tests.
func integrationTestConfig(t *testing.T) *config.Config {
	t.Helper()
	project := os.Getenv("NSELF_TEST_POSTGRES_PROJECT")
	if project == "" {
		project = "clipu"
	}
	return &config.Config{
		ProjectName: project,
		Postgres:    config.PostgresConfig{User: "postgres", DB: "postgres"},
	}
}

// resetSchemaVersionsTable drops np_common entirely so each subtest starts
// from a clean slate regardless of what an earlier subtest left behind.
func resetSchemaVersionsTable(t *testing.T, cfg *config.Config) {
	t.Helper()
	if err := execPSQL(context.Background(), cfg, `DROP SCHEMA IF EXISTS np_common CASCADE;`); err != nil {
		t.Fatalf("resetting np_common: %v", err)
	}
}

// mustCount runs a SELECT COUNT(*)-shaped query and fails the test if the
// result does not equal want. Shared by both scenarios below to keep each
// test function under the repo's per-function line cap.
func mustCount(t *testing.T, cfg *config.Config, sql, label, want string) {
	t.Helper()
	got, err := queryPSQL(context.Background(), cfg, sql)
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	if got != want {
		t.Fatalf("%s: got count=%s, want %s", label, got, want)
	}
}

// TestReconcileSchemaVersionsTable_FreshDB_Integration is scenario (a): no
// np_common.schema_versions exists yet. reconcileSchemaVersionsTable must
// create it with usable plugin/version columns, recordSchemaVersion must
// write successfully, and a repeat call must be a no-op (no duplicate row,
// no constraint error).
func TestReconcileSchemaVersionsTable_FreshDB_Integration(t *testing.T) {
	skipUnlessIntegration(t)
	cfg := integrationTestConfig(t)
	resetSchemaVersionsTable(t, cfg)
	ctx := context.Background()

	if err := reconcileSchemaVersionsTable(ctx, cfg); err != nil {
		t.Fatalf("reconcileSchemaVersionsTable on fresh DB: %v", err)
	}
	if err := recordSchemaVersion(ctx, cfg, "fresh-plugin", 1); err != nil {
		t.Fatalf("recordSchemaVersion: %v", err)
	}
	if v, err := getSchemaVersion(ctx, cfg, "fresh-plugin"); err != nil {
		t.Fatalf("getSchemaVersion: %v", err)
	} else if v != 1 {
		t.Fatalf("getSchemaVersion = %d, want 1", v)
	}

	// Second call must be a no-op, not a duplicate-row or constraint error.
	if err := recordSchemaVersion(ctx, cfg, "fresh-plugin", 1); err != nil {
		t.Fatalf("recordSchemaVersion (repeat): %v", err)
	}
	mustCount(t, cfg,
		`SELECT COUNT(*) FROM np_common.schema_versions WHERE plugin = 'np_fresh_plugin' AND version = 1;`,
		"row count after repeat recordSchemaVersion", "1")
}

// seedLegacySchemaVersions creates np_common.schema_versions in the
// migration-ledger's pre-fix shape (name TEXT, applied_at TIMESTAMPTZ) with
// two real rows — the exact shape prod evidence described: "columns name
// text not null, applied_at timestamptz not null default now()".
func seedLegacySchemaVersions(t *testing.T, cfg *config.Config) {
	t.Helper()
	legacyDDL := `CREATE SCHEMA np_common;
CREATE TABLE np_common.schema_versions (
  name TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO np_common.schema_versions (name) VALUES ('20260101_init.sql'), ('20260115_add_index.sql');`
	if err := execPSQL(context.Background(), cfg, legacyDDL); err != nil {
		t.Fatalf("seeding legacy schema_versions: %v", err)
	}
}

// TestReconcileSchemaVersionsTable_LegacyTable_Integration is scenario (b):
// np_common.schema_versions already exists in the migration ledger's legacy
// shape with real rows in it. reconcileSchemaVersionsTable must add the
// plugin/version columns and backfill plugin = name without losing those
// rows, and createPluginSchema's full flow must then succeed — including a
// second, no-op call, which is the exact "nself plugin update cron" prod
// failure this closes: update calls createPluginSchema again on an already
// -schema'd plugin, and that second call must not re-fail.
func TestReconcileSchemaVersionsTable_LegacyTable_Integration(t *testing.T) {
	skipUnlessIntegration(t)
	cfg := integrationTestConfig(t)
	resetSchemaVersionsTable(t, cfg)
	ctx := context.Background()
	seedLegacySchemaVersions(t, cfg)

	if err := reconcileSchemaVersionsTable(ctx, cfg); err != nil {
		t.Fatalf("reconcileSchemaVersionsTable on legacy table: %v", err)
	}

	mustCount(t, cfg,
		`SELECT COUNT(*) FROM np_common.schema_versions WHERE name IN ('20260101_init.sql', '20260115_add_index.sql');`,
		"legacy rows surviving reconciliation", "2")
	mustCount(t, cfg,
		`SELECT COUNT(*) FROM np_common.schema_versions WHERE plugin = name;`,
		"rows backfilled with plugin = name", "2")

	if err := createPluginSchema(ctx, cfg, "cron"); err != nil {
		t.Fatalf("createPluginSchema on reconciled legacy table: %v", err)
	}
	if err := createPluginSchema(ctx, cfg, "cron"); err != nil {
		t.Fatalf("createPluginSchema (repeat, must no-op): %v", err)
	}
	mustCount(t, cfg,
		`SELECT COUNT(*) FROM np_common.schema_versions WHERE plugin = 'np_cron' AND version = 1;`,
		"recorded version rows for cron", "1")
}
