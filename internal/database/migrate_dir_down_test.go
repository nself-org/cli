package database

// Purpose: tests for PendingDirMigrations and MigrateDownDir (P7-PROD-77).
// Two layers: (1) unit tests over a fake `docker` on PATH that records every
// statement, proving the preview issues SELECTs only and a rollback touches
// only the named directory migration; (2) an INTEGRATION=1 test against a real
// postgres container running the full acceptance flow.
// Inputs: temp migration directories, canned ledger answers, a scratch DB.
// Outputs: assertions over recorded statements and live ledger/schema state.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/config"
)

// dirFakeDocker records each call to $FAKE_DOCKER_DIR/calls.log: `-c`
// statements as EXEC, stdin batches as PIPE (saved as pipe-N.sql), `-tAc`
// queries as QUERY answered from q_* files.
const dirFakeDocker = `#!/bin/sh
D="$FAKE_DOCKER_DIR"
case "$*" in
  *" -i "*)
    n=$(ls "$D" | grep -c '^pipe-')
    cat > "$D/pipe-$n.sql"
    echo "CALL PIPE pipe-$n.sql" >> "$D/calls.log"
    if [ -f "$D/fail_pipe" ]; then echo "boom" >&2; exit 1; fi
    exit 0;;
esac
for last; do :; done
case "$*" in
  *" -c "*) echo "CALL EXEC $last" >> "$D/calls.log"; exit 0;;
esac
echo "CALL QUERY $last" >> "$D/calls.log"
case "$last" in
  *"to_regclass('np_common"*) cat "$D/q_legacy_exists" 2>/dev/null;;
  *"to_regclass('nself_ops"*) cat "$D/q_ops_exists" 2>/dev/null;;
  *"ORDER BY applied_at DESC"*) cat "$D/q_latest" 2>/dev/null;;
  *"name || '|' || applied_at"*) cat "$D/q_applied" 2>/dev/null;;
  *"name || '|' || checksum"*) cat "$D/q_checksums" 2>/dev/null;;
esac
exit 0
`

func fakeDockerState(t *testing.T, answers map[string]string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a /bin/sh script")
	}
	binDir, stateDir := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "docker"), []byte(dirFakeDocker), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DOCKER_DIR", stateDir)
	for name, body := range answers {
		if err := os.WriteFile(filepath.Join(stateDir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return stateDir
}

func recordedCalls(t *testing.T, stateDir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(stateDir, "calls.log"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func countPrefix(calls []string, prefix string) int {
	n := 0
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func mkMigDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

var dirTestFiles = map[string]string{
	"001_a.sql": "CREATE TABLE dir_a (id int);", "001_a.down.sql": "DROP TABLE dir_a;",
	"002_b.sql": "CREATE TABLE dir_b (id int);", "002_b_down.sql": "DROP TABLE dir_b;",
}

var fakeCfg = &config.Config{ProjectName: "fakeproj", Postgres: config.PostgresConfig{User: "postgres", DB: "nself"}}

func TestMigrateDirPending_IssuesSelectsOnly(t *testing.T) {
	dir := mkMigDir(t, dirTestFiles)
	sd := fakeDockerState(t, map[string]string{
		"q_legacy_exists": "yes", "q_ops_exists": "yes",
		"q_applied": "001_a.sql|2026-10-04T10:00:00Z",
	})
	got, err := PendingDirMigrations(context.Background(), fakeCfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "002_b.sql" {
		t.Fatalf("pending = %v, want [002_b.sql]", got)
	}
	calls := recordedCalls(t, sd)
	if countPrefix(calls, "CALL EXEC")+countPrefix(calls, "CALL PIPE") != 0 {
		t.Fatalf("preview wrote: %v", calls)
	}
	for _, c := range calls {
		if !strings.HasPrefix(c, "CALL QUERY SELECT ") {
			t.Errorf("non-SELECT statement in preview: %s", c)
		}
	}
}

func TestMigrateDirPending_MissingDirIsError(t *testing.T) {
	fakeDockerState(t, nil)
	if _, err := PendingDirMigrations(context.Background(), fakeCfg, filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("want error for a missing directory")
	}
}

func TestMigrateDirPending_LintFailureIsError(t *testing.T) {
	dir := mkMigDir(t, map[string]string{"001_x.sql": "ALTER TABLE t ADD CONSTRAINT IF NOT EXISTS c CHECK (true);"})
	fakeDockerState(t, map[string]string{"q_legacy_exists": "no", "q_ops_exists": "no"})
	if _, err := PendingDirMigrations(context.Background(), fakeCfg, dir); err == nil {
		t.Fatal("want the lint error the real run would raise")
	}
}

func TestMigrateDownDir_RevertsOnlyNamedMigration(t *testing.T) {
	dir := mkMigDir(t, dirTestFiles)
	sd := fakeDockerState(t, map[string]string{"q_latest": "002_b.sql\n"})
	got, err := MigrateDownDir(context.Background(), fakeCfg, dir, 1)
	if err != nil || len(got) != 1 || got[0] != "002_b.sql" {
		t.Fatalf("got %v, %v", got, err)
	}
	calls := recordedCalls(t, sd)
	if n := countPrefix(calls, "CALL PIPE"); n != 1 {
		t.Fatalf("pipes = %d, want 1", n)
	}
	b, _ := os.ReadFile(filepath.Join(sd, "pipe-0.sql"))
	s := string(b)
	if !strings.HasPrefix(s, "BEGIN;") || !strings.HasSuffix(s, "COMMIT;\n") ||
		!strings.Contains(s, "DROP TABLE dir_b;") || strings.Contains(s, "dir_a") ||
		strings.Count(s, "DELETE FROM") != 2 {
		t.Errorf("unexpected down transaction:\n%s", s)
	}
}

func TestMigrateDownDir_BothDownNamingStyles(t *testing.T) {
	dir := mkMigDir(t, dirTestFiles)
	sd := fakeDockerState(t, map[string]string{"q_latest": "002_b.sql\n001_a.sql\n"})
	got, err := MigrateDownDir(context.Background(), fakeCfg, dir, 2)
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
	a, _ := os.ReadFile(filepath.Join(sd, "pipe-1.sql"))
	if !strings.Contains(string(a), "DROP TABLE dir_a;") {
		t.Errorf("second step did not use 001_a.down.sql:\n%s", a)
	}
}

func TestMigrateDownDir_MissingDownFileNamesBothPaths(t *testing.T) {
	dir := mkMigDir(t, map[string]string{"003_c.sql": "SELECT 1;"})
	sd := fakeDockerState(t, map[string]string{"q_latest": "003_c.sql\n"})
	_, err := MigrateDownDir(context.Background(), fakeCfg, dir, 1)
	if err == nil || !strings.Contains(err.Error(), "003_c_down.sql") || !strings.Contains(err.Error(), "003_c.down.sql") {
		t.Fatalf("error must name both expected paths, got %v", err)
	}
	if countPrefix(recordedCalls(t, sd), "CALL PIPE") != 0 {
		t.Error("something was executed despite the missing down file")
	}
}

func TestMigrateDownDir_RefusesMigrationOutsideDir(t *testing.T) {
	dir := mkMigDir(t, dirTestFiles)
	sd := fakeDockerState(t, map[string]string{"q_latest": "900_other.sql\n"})
	_, err := MigrateDownDir(context.Background(), fakeCfg, dir, 1)
	if err == nil || !strings.Contains(err.Error(), "900_other.sql") {
		t.Fatalf("want refusal naming 900_other.sql, got %v", err)
	}
	if countPrefix(recordedCalls(t, sd), "CALL PIPE") != 0 {
		t.Error("reverted a migration outside the directory")
	}
}

// A later step with no down file must stop everything before step 1 runs.
func TestMigrateDownDir_ResolvesAllStepsBeforeRunning(t *testing.T) {
	files := map[string]string{"001_a.sql": "x", "002_b.sql": "x", "002_b_down.sql": "DROP TABLE dir_b;"}
	dir := mkMigDir(t, files)
	sd := fakeDockerState(t, map[string]string{"q_latest": "002_b.sql\n001_a.sql\n"})
	if _, err := MigrateDownDir(context.Background(), fakeCfg, dir, 2); err == nil {
		t.Fatal("want error: 001_a has no down file")
	}
	if countPrefix(recordedCalls(t, sd), "CALL PIPE") != 0 {
		t.Error("step 1 ran although step 2 could not")
	}
}

func TestMigrateDownDir_FewerAppliedThanStepsRevertsNothing(t *testing.T) {
	dir := mkMigDir(t, dirTestFiles)
	sd := fakeDockerState(t, map[string]string{"q_latest": "002_b.sql\n"})
	if _, err := MigrateDownDir(context.Background(), fakeCfg, dir, 3); err == nil {
		t.Fatal("want error when --steps exceeds applied migrations")
	}
	if countPrefix(recordedCalls(t, sd), "CALL PIPE") != 0 {
		t.Error("partial rollback executed")
	}
}

func TestMigrateDownDir_EmptyLedgerAndBadInputs(t *testing.T) {
	dir := mkMigDir(t, dirTestFiles)
	fakeDockerState(t, map[string]string{"q_latest": ""})
	ctx := context.Background()
	if _, err := MigrateDownDir(ctx, fakeCfg, dir, 1); err == nil {
		t.Error("empty ledger must be an error")
	}
	if _, err := MigrateDownDir(ctx, fakeCfg, dir, 0); err == nil {
		t.Error("steps 0 must be an error")
	}
	if _, err := MigrateDownDir(ctx, fakeCfg, filepath.Join(dir, "nope"), 1); err == nil {
		t.Error("missing directory must be an error")
	}
}

func TestMigrateDownDir_RefusesDownFileWithOwnTransactionControl(t *testing.T) {
	dir := mkMigDir(t, map[string]string{"001_a.sql": "x", "001_a.down.sql": "DROP TABLE dir_a;\nCOMMIT;\n"})
	sd := fakeDockerState(t, map[string]string{"q_latest": "001_a.sql\n"})
	if _, err := MigrateDownDir(context.Background(), fakeCfg, dir, 1); err == nil {
		t.Fatal("want refusal")
	}
	if countPrefix(recordedCalls(t, sd), "CALL PIPE") != 0 {
		t.Error("executed a down file that escapes the transaction")
	}
}

func TestMigrateDownDir_PsqlFailureIsNotSuccess(t *testing.T) {
	dir := mkMigDir(t, dirTestFiles)
	fakeDockerState(t, map[string]string{"q_latest": "002_b.sql\n", "fail_pipe": ""})
	got, err := MigrateDownDir(context.Background(), fakeCfg, dir, 1)
	if err == nil || len(got) != 0 {
		t.Fatalf("failed revert reported success: %v, %v", got, err)
	}
}

func TestMigrateDownDir_QuotesLedgerName(t *testing.T) {
	// validateMigrationName already blocks quotes; the SQL builder also doubles
	// them, so a future relaxed validator cannot open an injection.
	s := downTxSQL(downStep{name: "a'b.sql", sql: "SELECT 1;"})
	if !strings.Contains(s, "name = 'a''b.sql'") {
		t.Errorf("name not quoted:\n%s", s)
	}
}

// --- real Postgres acceptance flow (INTEGRATION=1) ---

func startDirPG(t *testing.T) *config.Config {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run Docker Postgres integration tests")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found in PATH")
	}
	project := fmt.Sprintf("p77dir%d", time.Now().UnixNano())
	container := project + "_postgres"
	if out, err := exec.Command("docker", "run", "-d", "--name", container,
		"-e", "POSTGRES_PASSWORD=test_integration_pw", "-e", "POSTGRES_DB=nself",
		"postgres:16-alpine").CombinedOutput(); err != nil {
		t.Fatalf("docker run: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", container).Run() })
	deadline := time.Now().Add(45 * time.Second)
	for exec.Command("docker", "exec", container, "psql", "-U", "postgres", "-d", "nself", "-c", "SELECT 1").Run() != nil {
		if time.Now().After(deadline) {
			t.Fatal("postgres never became ready")
		}
		time.Sleep(500 * time.Millisecond)
	}
	return &config.Config{ProjectName: project, Postgres: config.PostgresConfig{User: "postgres", DB: "nself"}}
}

func pgScalar(t *testing.T, cfg *config.Config, q string) string {
	t.Helper()
	out, err := querySQL(context.Background(), cfg, "nself", q)
	if err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	return strings.TrimSpace(out)
}

func TestMigrateDirIntegration_DryRunUpDownFlow(t *testing.T) {
	cfg := startDirPG(t)
	ctx := context.Background()
	dir := mkMigDir(t, dirTestFiles)

	// 1. dry-run on a database that has never been migrated: lists both,
	// creates nothing (not even the ledger tables).
	pending, err := PendingDirMigrations(ctx, cfg, dir)
	if err != nil || len(pending) != 2 {
		t.Fatalf("dry-run pending = %v, %v", pending, err)
	}
	if got := pgScalar(t, cfg, "SELECT count(*) FROM pg_class WHERE relname IN ('schema_versions','migrations','dir_a','dir_b')"); got != "0" {
		t.Fatalf("dry-run created %s relation(s)", got)
	}

	// 2. up applies both; a second dry-run lists none.
	if n, err := MigrateUpDir(ctx, cfg, dir); err != nil || n != 2 {
		t.Fatalf("up = %d, %v", n, err)
	}
	if pending, err = PendingDirMigrations(ctx, cfg, dir); err != nil || len(pending) != 0 {
		t.Fatalf("dry-run after up = %v, %v", pending, err)
	}
	ledger := func() string {
		return pgScalar(t, cfg, "SELECT (SELECT string_agg(name, ',' ORDER BY name) FROM np_common.schema_versions) || ' / ' || (SELECT string_agg(name, ',' ORDER BY name) FROM nself_ops.migrations)")
	}
	before := ledger()

	// 3. dry-run on an applied ledger changes neither ledger.
	_, _ = PendingDirMigrations(ctx, cfg, dir)
	if after := ledger(); after != before {
		t.Fatalf("dry-run changed the ledger: %q -> %q", before, after)
	}

	// 4. down --steps 1 reverts only 002_b.
	got, err := MigrateDownDir(ctx, cfg, dir, 1)
	if err != nil || len(got) != 1 || got[0] != "002_b.sql" {
		t.Fatalf("down 1 = %v, %v", got, err)
	}
	if ledger() != "001_a.sql / 001_a.sql" {
		t.Fatalf("ledger after first down = %q", ledger())
	}
	if pgScalar(t, cfg, "SELECT to_regclass('dir_b') IS NULL") != "t" || pgScalar(t, cfg, "SELECT to_regclass('dir_a') IS NOT NULL") != "t" {
		t.Fatal("down touched the wrong table")
	}

	// 5. a second down reverts the other, leaving both ledgers empty.
	if got, err = MigrateDownDir(ctx, cfg, dir, 1); err != nil || got[0] != "001_a.sql" {
		t.Fatalf("down 2 = %v, %v", got, err)
	}
	if pgScalar(t, cfg, "SELECT count(*) FROM np_common.schema_versions") != "0" ||
		pgScalar(t, cfg, "SELECT count(*) FROM nself_ops.migrations") != "0" {
		t.Fatal("ledgers not empty after both reverts")
	}

	// 6. a failing down file rolls its transaction back: table and ledger stay.
	if n, err := MigrateUpDir(ctx, cfg, dir); err != nil || n != 2 {
		t.Fatalf("re-up = %d, %v", n, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "002_b_down.sql"), []byte("DROP TABLE dir_b;\nDROP TABLE does_not_exist;"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err = MigrateDownDir(ctx, cfg, dir, 1); err == nil || len(got) != 0 {
		t.Fatalf("failing down reported %v, %v", got, err)
	}
	if pgScalar(t, cfg, "SELECT to_regclass('dir_b') IS NOT NULL") != "t" ||
		pgScalar(t, cfg, "SELECT count(*) FROM np_common.schema_versions WHERE name = '002_b.sql'") != "1" ||
		pgScalar(t, cfg, "SELECT count(*) FROM nself_ops.migrations WHERE name = '002_b.sql'") != "1" {
		t.Fatal("failed down was not rolled back as one transaction")
	}
}
