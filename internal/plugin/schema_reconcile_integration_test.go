//go:build integration

// schema_reconcile_integration_test.go — live-Postgres verification for
// ensurePluginSchemaVersionsTable against every np_common.schema_versions
// shape seen in production: absent, the migration ledger's real DDL (name
// TEXT PRIMARY KEY, copied from internal/database/migrate_sql.go, not
// paraphrased; the 1.4.9 test seeded a NOT NULL column without the key and
// missed the prod failure), and the table as the 1.4.9 reconcile left it.
//
// Run with:
//
//	INTEGRATION=1 go test -tags integration -timeout 120s \
//	    ./internal/plugin/... -run TestPluginSchemaVersions
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

// ledgerDDL is internal/database/migrate_sql.go's ensureSchemaVersions DDL,
// verbatim.
const ledgerDDL = `CREATE SCHEMA IF NOT EXISTS np_common; CREATE TABLE IF NOT EXISTS np_common.schema_versions (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`

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

// TestPluginSchemaVersions_FreshDB_Integration: nothing exists yet. The
// plugin table is created, a record/repeat pair leaves one row, and the
// migration ledger's table is NOT created by this package.
func TestPluginSchemaVersions_FreshDB_Integration(t *testing.T) {
	skipUnlessIntegration(t)
	cfg := integrationTestConfig(t)
	resetSchemaVersionsTable(t, cfg)
	ctx := context.Background()

	if err := ensurePluginSchemaVersionsTable(ctx, cfg); err != nil {
		t.Fatalf("ensurePluginSchemaVersionsTable on fresh DB: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := recordSchemaVersion(ctx, cfg, "fresh-plugin", 1); err != nil {
			t.Fatalf("recordSchemaVersion #%d: %v", i+1, err)
		}
	}
	if v, err := getSchemaVersion(ctx, cfg, "fresh-plugin"); err != nil || v != 1 {
		t.Fatalf("getSchemaVersion = %d, %v; want 1, nil", v, err)
	}
	mustCount(t, cfg,
		`SELECT COUNT(*) FROM np_common.plugin_schema_versions WHERE plugin = 'np_fresh_plugin' AND version = 1;`,
		"row count after repeat recordSchemaVersion", "1")
	mustCount(t, cfg,
		`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'np_common' AND table_name = 'schema_versions';`,
		"ledger table created by the plugin package", "0")
}

// TestPluginSchemaVersions_LedgerTable_Integration: the migration ledger ran
// first (nself-web prod). createPluginSchema must succeed twice and leave the
// ledger table's shape and rows exactly as they were.
func TestPluginSchemaVersions_LedgerTable_Integration(t *testing.T) {
	skipUnlessIntegration(t)
	cfg := integrationTestConfig(t)
	resetSchemaVersionsTable(t, cfg)
	ctx := context.Background()
	seed := ledgerDDL + `; INSERT INTO np_common.schema_versions (name) VALUES ('20260101_init.sql'), ('20260115_add_index.sql');`
	if err := execPSQL(ctx, cfg, seed); err != nil {
		t.Fatalf("seeding ledger table: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := createPluginSchema(ctx, cfg, "cron"); err != nil {
			t.Fatalf("createPluginSchema #%d on ledger-owned table: %v", i+1, err)
		}
	}
	mustCount(t, cfg,
		`SELECT COUNT(*) FROM np_common.plugin_schema_versions WHERE plugin = 'np_cron' AND version = 1;`,
		"recorded version rows for cron", "1")
	mustCount(t, cfg,
		`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = 'np_common' AND table_name = 'schema_versions';`,
		"ledger table column count (unaltered)", "2")
	mustCount(t, cfg,
		`SELECT COUNT(*) FROM np_common.schema_versions;`,
		"ledger rows (unaltered)", "2")
}

// TestPluginSchemaVersions_After149Reconcile_Integration: the table as the
// 1.4.9 in-place reconcile left it (plugin/version columns added, ledger
// rows backfilled plugin = name with no version) plus one real plugin row.
// Only the real plugin row is carried over, so cron is not re-provisioned.
func TestPluginSchemaVersions_After149Reconcile_Integration(t *testing.T) {
	skipUnlessIntegration(t)
	cfg := integrationTestConfig(t)
	resetSchemaVersionsTable(t, cfg)
	ctx := context.Background()
	seed := ledgerDDL + `;
ALTER TABLE np_common.schema_versions ADD COLUMN plugin TEXT, ADD COLUMN version INT;
INSERT INTO np_common.schema_versions (name, plugin) VALUES ('20260101_init.sql', '20260101_init.sql');
INSERT INTO np_common.schema_versions (name, plugin, version) VALUES ('np_cron', 'np_cron', 1);`
	if err := execPSQL(ctx, cfg, seed); err != nil {
		t.Fatalf("seeding 1.4.9-reconciled table: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := ensurePluginSchemaVersionsTable(ctx, cfg); err != nil {
			t.Fatalf("ensurePluginSchemaVersionsTable #%d: %v", i+1, err)
		}
	}
	mustCount(t, cfg, `SELECT COUNT(*) FROM np_common.plugin_schema_versions;`,
		"carried-over rows (ledger rows excluded)", "1")
	if v, err := getSchemaVersion(ctx, cfg, "cron"); err != nil || v != 1 {
		t.Fatalf("getSchemaVersion(cron) = %d, %v; want 1, nil", v, err)
	}
}

// TestPluginSchemaVersions_NoAppliedAt_Integration: an old table with plugin
// and version but no applied_at column still carries its rows over.
func TestPluginSchemaVersions_NoAppliedAt_Integration(t *testing.T) {
	skipUnlessIntegration(t)
	cfg := integrationTestConfig(t)
	resetSchemaVersionsTable(t, cfg)
	ctx := context.Background()
	seed := `CREATE SCHEMA np_common;
CREATE TABLE np_common.schema_versions (plugin TEXT, version INT);
INSERT INTO np_common.schema_versions VALUES ('np_notify', 1), ('np_notify', 1);`
	if err := execPSQL(ctx, cfg, seed); err != nil {
		t.Fatalf("seeding table without applied_at: %v", err)
	}
	if err := ensurePluginSchemaVersionsTable(ctx, cfg); err != nil {
		t.Fatalf("ensurePluginSchemaVersionsTable: %v", err)
	}
	mustCount(t, cfg, `SELECT COUNT(*) FROM np_common.plugin_schema_versions WHERE plugin = 'np_notify';`,
		"carried-over rows without applied_at", "1")
}

// TestPluginSchemaVersions_RemoveThenReinstall_Integration: remove drops the
// schema and role; a reinstall must provision them again rather than trust a
// version row left behind.
func TestPluginSchemaVersions_RemoveThenReinstall_Integration(t *testing.T) {
	skipUnlessIntegration(t)
	cfg := integrationTestConfig(t)
	resetSchemaVersionsTable(t, cfg)
	ctx := context.Background()

	if err := createPluginSchema(ctx, cfg, "reinstall-probe"); err != nil {
		t.Fatalf("createPluginSchema: %v", err)
	}
	if err := dropPluginSchema(ctx, cfg, "reinstall-probe"); err != nil {
		t.Fatalf("dropPluginSchema: %v", err)
	}
	mustCount(t, cfg, `SELECT COUNT(*) FROM np_common.plugin_schema_versions WHERE plugin = 'np_reinstall_probe';`,
		"version rows after remove", "0")
	if err := createPluginSchema(ctx, cfg, "reinstall-probe"); err != nil {
		t.Fatalf("createPluginSchema after remove: %v", err)
	}
	mustCount(t, cfg, `SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = 'np_reinstall_probe';`,
		"schema present after reinstall", "1")
	if err := dropPluginSchema(ctx, cfg, "reinstall-probe"); err != nil {
		t.Fatalf("cleanup dropPluginSchema: %v", err)
	}
}
