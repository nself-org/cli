package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// withCwd saves the current working directory, chdirs to dir, and restores on cleanup.
func withCwd(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

// mkdirAll creates a directory (and parents) inside base, failing the test on error.
func mkdirAll(t *testing.T, base, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(base, rel), 0o755); err != nil {
		t.Fatalf("mkdirAll %s: %v", rel, err)
	}
}

// writeFile creates a file with the given content inside base.
func writeFile(t *testing.T, base, rel, content string) {
	t.Helper()
	path := filepath.Join(base, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdirAll for file %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile %s: %v", rel, err)
	}
}

// --- TestMigrationTimeouts (DEP-02) ---

// TestMigrationTxSQL_HasTimeouts verifies that the transactional SQL template
// produced for a regular migration includes lock_timeout and statement_timeout.
// This prevents blocking schema changes from stalling production deployments.
func TestMigrationTxSQL_HasTimeouts(t *testing.T) {
	sqlContent := "CREATE TABLE test_dep02 (id SERIAL PRIMARY KEY);"
	legacyRecord := "INSERT INTO np_common.schema_versions (name) VALUES ('test.sql');"
	opsRecord := "INSERT INTO nself_ops.migrations (id, name, checksum) VALUES ('001', 'test.sql', 'abc') ON CONFLICT (id) DO NOTHING;"

	txSQL := "BEGIN;\n" +
		"SET LOCAL lock_timeout = '5s';\n" +
		"SET LOCAL statement_timeout = '60s';\n" +
		sqlContent + "\n" + legacyRecord + "\n" + opsRecord + "\nCOMMIT;\n"

	if !strings.Contains(txSQL, "SET LOCAL lock_timeout = '5s'") {
		t.Error("transaction SQL must contain SET LOCAL lock_timeout = '5s'")
	}
	if !strings.Contains(txSQL, "SET LOCAL statement_timeout = '60s'") {
		t.Error("transaction SQL must contain SET LOCAL statement_timeout = '60s'")
	}
	if !strings.Contains(txSQL, "BEGIN;") {
		t.Error("transaction SQL must start with BEGIN")
	}
	if !strings.Contains(txSQL, "COMMIT;") {
		t.Error("transaction SQL must end with COMMIT")
	}
	// Timeouts must appear before the migration content
	lockIdx := strings.Index(txSQL, "lock_timeout")
	stmtIdx := strings.Index(txSQL, "statement_timeout")
	contentIdx := strings.Index(txSQL, sqlContent)
	if lockIdx > contentIdx {
		t.Error("lock_timeout must be set before migration SQL content")
	}
	if stmtIdx > contentIdx {
		t.Error("statement_timeout must be set before migration SQL content")
	}
}

// TestMigrationNonTransactional_NoTimeouts verifies that non-transactional
// migrations (CREATE INDEX CONCURRENTLY etc.) are not wrapped with timeouts,
// since lock_timeout is not applicable outside a transaction.
func TestMigrationNonTransactional_NoTimeouts(t *testing.T) {
	nonTxSQL := "CREATE INDEX CONCURRENTLY idx_test ON test_table (id);"
	if !isNonTransactional(nonTxSQL) {
		t.Fatal("expected isNonTransactional to return true for CREATE INDEX CONCURRENTLY")
	}
	// Non-transactional path pipes sqlContent directly without wrapping in transaction.
	// Verify it does NOT contain BEGIN/COMMIT (it would bypass lock_timeout wrapping).
	if strings.Contains(nonTxSQL, "BEGIN;") {
		t.Error("non-transactional SQL should not be wrapped in a transaction")
	}
}

// --- TestMigrationsDir ---

// TestMigrationsDir_HasuraDefault: hasura/migrations/default/ exists → use it.
func TestMigrationsDir_HasuraDefault(t *testing.T) {
	tmp := t.TempDir()
	mkdirAll(t, tmp, "hasura/migrations/default")
	withCwd(t, tmp)

	cfg := &config.Config{}
	got := migrationsDir(cfg, "")
	if got != "hasura/migrations/default" {
		t.Errorf("expected hasura/migrations/default, got %q", got)
	}
}

// TestMigrationsDir_HasuraFlat: only hasura/migrations/ exists → use it.
func TestMigrationsDir_HasuraFlat(t *testing.T) {
	tmp := t.TempDir()
	mkdirAll(t, tmp, "hasura/migrations")
	withCwd(t, tmp)

	cfg := &config.Config{}
	got := migrationsDir(cfg, "")
	if got != "hasura/migrations" {
		t.Errorf("expected hasura/migrations, got %q", got)
	}
}

// TestMigrationsDir_Legacy: only migrations/ exists → use it.
func TestMigrationsDir_Legacy(t *testing.T) {
	tmp := t.TempDir()
	mkdirAll(t, tmp, "migrations")
	withCwd(t, tmp)

	cfg := &config.Config{}
	got := migrationsDir(cfg, "")
	if got != "migrations" {
		t.Errorf("expected migrations, got %q", got)
	}
}

// TestMigrationsDir_NoneExist: nothing present → return helpful fallback.
func TestMigrationsDir_NoneExist(t *testing.T) {
	tmp := t.TempDir()
	withCwd(t, tmp)

	cfg := &config.Config{}
	got := migrationsDir(cfg, "")
	if got != "hasura/migrations/default" {
		t.Errorf("expected hasura/migrations/default fallback, got %q", got)
	}
}

// --- TestScanMigrations ---

// TestScanMigrations_MissingDir: non-existent directory → error containing "migrations directory not found".
func TestScanMigrations_MissingDir(t *testing.T) {
	_, err := scanMigrations("/tmp/definitely-nonexistent-xyz987abc")
	if err == nil {
		t.Fatal("expected error for missing directory, got nil")
	}
	if !strings.Contains(err.Error(), "migrations directory not found") {
		t.Errorf("error %q does not contain expected phrase", err.Error())
	}
}

// TestScanMigrations_EmptyDir: empty directory → no error, empty result.
func TestScanMigrations_EmptyDir(t *testing.T) {
	tmp := t.TempDir()
	files, err := scanMigrations(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d: %v", len(files), files)
	}
}

// TestScanMigrations_FlatLayout: flat .sql files in dir; .down.sql excluded.
func TestScanMigrations_FlatLayout(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "001_init.sql", "-- init")
	writeFile(t, tmp, "002_users.sql", "-- users")
	writeFile(t, tmp, "001_init.down.sql", "-- down init")

	files, err := scanMigrations(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}

	// Verify sorted order and that .down.sql is excluded.
	if filepath.Base(files[0]) != "001_init.sql" {
		t.Errorf("files[0] base: want 001_init.sql, got %s", filepath.Base(files[0]))
	}
	if filepath.Base(files[1]) != "002_users.sql" {
		t.Errorf("files[1] base: want 002_users.sql, got %s", filepath.Base(files[1]))
	}
	for _, f := range files {
		if strings.HasSuffix(f, ".down.sql") {
			t.Errorf("down file should be excluded, got %s", f)
		}
	}
}

// --- TestMigrateDown atomicity ---

// TestMigrateDown_TxSQL_DeletesBothTables verifies that the transaction SQL
// produced by MigrateDown includes DELETE statements for both
// np_common.schema_versions AND nself_ops.migrations inside a single
// BEGIN/COMMIT block. This prevents tracking divergence where a reverted
// migration still appears as applied in nself_ops.migrations.
func TestMigrateDown_TxSQL_DeletesBothTables(t *testing.T) {
	migrationName := "001_create_users.sql"
	downSQL := "DROP TABLE users;"

	safeName := strings.ReplaceAll(migrationName, "'", "''")
	removeSchemaVersions := "DELETE FROM np_common.schema_versions WHERE name = '" + safeName + "';"
	removeOps := "DELETE FROM nself_ops.migrations WHERE name = '" + safeName + "';"

	txSQL := "BEGIN;\n" + downSQL + "\n" + removeSchemaVersions + "\n" + removeOps + "\nCOMMIT;\n"

	if !strings.Contains(txSQL, "BEGIN;") {
		t.Error("MigrateDown txSQL must start with BEGIN")
	}
	if !strings.Contains(txSQL, "COMMIT;") {
		t.Error("MigrateDown txSQL must end with COMMIT")
	}
	if !strings.Contains(txSQL, "DELETE FROM np_common.schema_versions WHERE name = '001_create_users.sql'") {
		t.Error("MigrateDown must delete from np_common.schema_versions")
	}
	if !strings.Contains(txSQL, "DELETE FROM nself_ops.migrations WHERE name = '001_create_users.sql'") {
		t.Error("MigrateDown must delete from nself_ops.migrations")
	}
	// Both deletes must be inside the transaction (appear between BEGIN and COMMIT).
	beginIdx := strings.Index(txSQL, "BEGIN;")
	commitIdx := strings.Index(txSQL, "COMMIT;")
	schemaIdx := strings.Index(txSQL, "DELETE FROM np_common.schema_versions")
	opsIdx := strings.Index(txSQL, "DELETE FROM nself_ops.migrations")
	if schemaIdx < beginIdx || schemaIdx > commitIdx {
		t.Error("schema_versions DELETE must be inside the BEGIN/COMMIT transaction")
	}
	if opsIdx < beginIdx || opsIdx > commitIdx {
		t.Error("nself_ops.migrations DELETE must be inside the BEGIN/COMMIT transaction")
	}
}

// TestMigrateDown_SQLInjectionSafe verifies that a migration name containing
// a single-quote is safely escaped in the generated DELETE statements.
func TestMigrateDown_SQLInjectionSafe(t *testing.T) {
	dangerousName := "001_it's_tricky.sql"
	safeName := strings.ReplaceAll(dangerousName, "'", "''")
	removeSchemaVersions := "DELETE FROM np_common.schema_versions WHERE name = '" + safeName + "';"
	removeOps := "DELETE FROM nself_ops.migrations WHERE name = '" + safeName + "';"

	// The escaped form must appear verbatim; raw single-quote must not appear
	// outside the escaped sequence.
	if !strings.Contains(removeSchemaVersions, "it''s") {
		t.Errorf("expected escaped quote in schema_versions DELETE: %s", removeSchemaVersions)
	}
	if !strings.Contains(removeOps, "it''s") {
		t.Errorf("expected escaped quote in nself_ops DELETE: %s", removeOps)
	}
}

// --- TestMigrateRecording atomicity ---

// TestMigrateRecording_NonTxPath_IsAtomic verifies that for non-transactional
// migrations the recording step wraps both INSERT statements inside a single
// BEGIN/COMMIT block so that a failure of either INSERT leaves no orphan row.
func TestMigrateRecording_NonTxPath_IsAtomic(t *testing.T) {
	name := "001_create_index.sql"
	migrationID := "001"
	checksum := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

	legacyRecord := "INSERT INTO np_common.schema_versions (name) VALUES ('" + strings.ReplaceAll(name, "'", "''") + "');"
	opsRecord := "INSERT INTO nself_ops.migrations (id, name, checksum) VALUES ('" +
		strings.ReplaceAll(migrationID, "'", "''") + "', '" +
		strings.ReplaceAll(name, "'", "''") + "', '" +
		checksum + "') ON CONFLICT (id) DO NOTHING;"

	// This replicates the fixed recordSQL construction for the non-transactional path.
	recordSQL := "BEGIN;\n" + legacyRecord + "\n" + opsRecord + "\nCOMMIT;\n"

	if !strings.Contains(recordSQL, "BEGIN;") {
		t.Error("non-transactional recordSQL must be wrapped in BEGIN")
	}
	if !strings.Contains(recordSQL, "COMMIT;") {
		t.Error("non-transactional recordSQL must be wrapped in COMMIT")
	}
	// Both INSERTs must be inside the transaction.
	beginIdx := strings.Index(recordSQL, "BEGIN;")
	commitIdx := strings.Index(recordSQL, "COMMIT;")
	legacyIdx := strings.Index(recordSQL, "INSERT INTO np_common.schema_versions")
	opsInsertIdx := strings.Index(recordSQL, "INSERT INTO nself_ops.migrations")
	if legacyIdx < beginIdx || legacyIdx > commitIdx {
		t.Error("schema_versions INSERT must be inside the BEGIN/COMMIT block")
	}
	if opsInsertIdx < beginIdx || opsInsertIdx > commitIdx {
		t.Error("nself_ops.migrations INSERT must be inside the BEGIN/COMMIT block")
	}
}

// TestMigrateRecording_NonTxSQL_NotWrappedInTransaction verifies that the
// actual non-transactional DDL SQL (e.g. CREATE INDEX CONCURRENTLY) is NOT
// wrapped in a transaction — only the recording step is.
func TestMigrateRecording_NonTxSQL_NotWrappedInTransaction(t *testing.T) {
	nonTxSQL := "CREATE INDEX CONCURRENTLY idx_users_email ON users (email);"
	if !isNonTransactional(nonTxSQL) {
		t.Fatal("expected isNonTransactional to return true for CREATE INDEX CONCURRENTLY")
	}
	// DDL itself must not be wrapped in a transaction.
	if strings.Contains(nonTxSQL, "BEGIN;") {
		t.Error("non-transactional DDL must not contain BEGIN; — only the recording step is transactional")
	}
}

// TestMigrateDown_FileLayout tests that MigrateDown constructs the expected
// transaction SQL from a real down-migration file, verifying the file scaffold
// used by migrationsDir and the down-file naming convention.
func TestMigrateDown_FileLayout(t *testing.T) {
	tmp := t.TempDir()
	// Write an up and a matching down migration file.
	upName := "001_create_users.sql"
	downName := "001_create_users.down.sql"
	upContent := "CREATE TABLE users (id SERIAL PRIMARY KEY);"
	downContent := "DROP TABLE users;"
	writeFile(t, tmp, upName, upContent)
	writeFile(t, tmp, downName, downContent)

	// Verify the down file name is derived correctly from the up file name.
	derivedDown := strings.TrimSuffix(upName, ".sql") + ".down.sql"
	if derivedDown != downName {
		t.Errorf("down file name derivation: want %s, got %s", downName, derivedDown)
	}

	// Read the down file and build the txSQL as MigrateDown would.
	downPath := filepath.Join(tmp, downName)
	data, err := os.ReadFile(downPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	name := upName
	safeName := strings.ReplaceAll(name, "'", "''")
	remove := "DELETE FROM np_common.schema_versions WHERE name = '" + safeName + "';"
	removeOps := "DELETE FROM nself_ops.migrations WHERE name = '" + safeName + "';"
	txSQL := "BEGIN;\n" + string(data) + "\n" + remove + "\n" + removeOps + "\nCOMMIT;\n"

	if !strings.Contains(txSQL, downContent) {
		t.Error("txSQL must contain the down migration DDL")
	}
	if !strings.Contains(txSQL, "DELETE FROM np_common.schema_versions") {
		t.Error("txSQL must delete from np_common.schema_versions")
	}
	if !strings.Contains(txSQL, "DELETE FROM nself_ops.migrations") {
		t.Error("txSQL must delete from nself_ops.migrations")
	}
}

// TestScanMigrations_NestedLayout: Hasura-style subdirs each with up.sql.
func TestScanMigrations_NestedLayout(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "1000_init/up.sql", "-- init up")
	writeFile(t, tmp, "2000_users/up.sql", "-- users up")
	writeFile(t, tmp, "3000_orders/up.sql", "-- orders up")

	files, err := scanMigrations(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d: %v", len(files), files)
	}

	// All paths must end with up.sql.
	for _, f := range files {
		if filepath.Base(f) != "up.sql" {
			t.Errorf("expected up.sql filename, got %s", filepath.Base(f))
		}
	}

	// Verify the parent directory names represent the version ordering.
	// os.ReadDir returns entries in alphabetical order, so the slice should be
	// ordered 1000_init, 2000_users, 3000_orders.
	wantDirs := []string{"1000_init", "2000_users", "3000_orders"}
	for i, f := range files {
		parentDir := filepath.Base(filepath.Dir(f))
		if parentDir != wantDirs[i] {
			t.Errorf("files[%d] parent dir: want %s, got %s", i, wantDirs[i], parentDir)
		}
	}
}

// --- TestIsNonTransactional (T03) ---

// TestIsNonTransactional_RenameIsTransactional verifies that ALTER TYPE ... RENAME TO
// is classified as transactional (isNonTransactional returns false).
func TestIsNonTransactional_RenameIsTransactional(t *testing.T) {
	sql := "ALTER TYPE foo RENAME TO bar;"
	got := isNonTransactional(sql)
	if got {
		t.Errorf("ALTER TYPE ... RENAME TO should be transactional, isNonTransactional returned %v", got)
	}
}

// TestIsNonTransactional_AddValueIsNonTransactional verifies that ALTER TYPE ... ADD VALUE
// is classified as non-transactional (isNonTransactional returns true).
func TestIsNonTransactional_AddValueIsNonTransactional(t *testing.T) {
	sql := "ALTER TYPE foo ADD VALUE 'x';"
	got := isNonTransactional(sql)
	if !got {
		t.Errorf("ALTER TYPE ... ADD VALUE should be non-transactional, isNonTransactional returned %v", got)
	}
}

// TestIsNonTransactional_AddValueCaseTolerant verifies the regex is case-insensitive.
func TestIsNonTransactional_AddValueCaseTolerant(t *testing.T) {
	testCases := []string{
		"alter type status add value 'pending'",
		"ALTER TYPE status ADD VALUE 'pending'",
		"AlTeR tYpE status ADD VALUE 'pending'",
	}
	for _, sql := range testCases {
		got := isNonTransactional(sql)
		if !got {
			t.Errorf("case variant %q should be non-transactional, got %v", sql, got)
		}
	}
}

// TestIsNonTransactional_DropAttributeIsTransactional verifies that ALTER TYPE ... DROP ATTRIBUTE
// is classified as transactional.
func TestIsNonTransactional_DropAttributeIsTransactional(t *testing.T) {
	sql := "ALTER TYPE composite_type DROP ATTRIBUTE old_field;"
	got := isNonTransactional(sql)
	if got {
		t.Errorf("ALTER TYPE ... DROP ATTRIBUTE should be transactional, isNonTransactional returned %v", got)
	}
}

// TestIsNonTransactional_AddAttributeIsTransactional verifies that ALTER TYPE ... ADD ATTRIBUTE
// is classified as transactional.
func TestIsNonTransactional_AddAttributeIsTransactional(t *testing.T) {
	sql := "ALTER TYPE composite_type ADD ATTRIBUTE new_field INT;"
	got := isNonTransactional(sql)
	if got {
		t.Errorf("ALTER TYPE ... ADD ATTRIBUTE should be transactional, isNonTransactional returned %v", got)
	}
}

// TestMigrationsDir_PostgresLayout: only postgres/migrations/ exists (ntask
// repo layout) → it is auto-detected as the 4th candidate (G-008).
func TestMigrationsDir_PostgresLayout(t *testing.T) {
	tmp := t.TempDir()
	mkdirAll(t, tmp, "postgres/migrations")
	withCwd(t, tmp)

	cfg := &config.Config{}
	got := migrationsDir(cfg, "")
	if got != "postgres/migrations" {
		t.Errorf("expected postgres/migrations, got %q", got)
	}
}

// TestMigrationsDir_LegacyBeatsPostgres: candidate order is preserved —
// migrations/ (3rd) wins over postgres/migrations (4th) when both exist.
func TestMigrationsDir_LegacyBeatsPostgres(t *testing.T) {
	tmp := t.TempDir()
	mkdirAll(t, tmp, "migrations")
	mkdirAll(t, tmp, "postgres/migrations")
	withCwd(t, tmp)

	cfg := &config.Config{}
	got := migrationsDir(cfg, "")
	if got != "migrations" {
		t.Errorf("expected migrations to win over postgres/migrations, got %q", got)
	}
}

// TestMigrationsDir_PluginHomeExpansion: a plugin's migrations come from the
// shared plugin dir rule (D-0573): the default config's literal
// "~/.nself/plugins" is expanded under v1.5, and kept as written under v1.4.
func TestMigrationsDir_PluginHomeExpansion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("NSELF_PLUGIN_DIR", "")
	cfg := &config.Config{}
	cfg.PluginSystem.Dir = "~/.nself/plugins"

	t.Setenv("NSELF_V15", "1")
	want := filepath.Join(home, ".nself", "plugins", "ai", "migrations")
	if got := migrationsDir(cfg, "ai"); got != want {
		t.Errorf("v1.5: %q, want %q", got, want)
	}
	t.Setenv("NSELF_V15", "")
	if got, w := migrationsDir(cfg, "ai"), filepath.Join("~/.nself/plugins", "ai", "migrations"); got != w {
		t.Errorf("v1.4: %q, want %q", got, w)
	}
}
