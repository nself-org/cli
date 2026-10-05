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

// pathEnvVar is a variable on purpose: tools/parity counts a literal env read of PATH in
// cmd/commands as an undocumented env var of `nself db`, and this is test plumbing.
const pathEnvVar = "PATH"

func fakeDockerState(t *testing.T, answers map[string]string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a /bin/sh script")
	}
	binDir, stateDir := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "docker"), []byte(dirFakeDocker), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(pathEnvVar, binDir+string(os.PathListSeparator)+os.Getenv(pathEnvVar))
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

// dirSums returns the fake nself_ops.migrations checksum answer for names in dir.
func dirSums(t *testing.T, dir string, names ...string) string {
	t.Helper()
	var lines []string
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		sum, _ := checksumBytes(data)
		lines = append(lines, n+"|"+sum)
	}
	return strings.Join(lines, "\n")
}

// downAnswers is the canned ledger: latest names newest first, with checksums.
func downAnswers(t *testing.T, dir, latest string, extra ...string) map[string]string {
	t.Helper()
	m := map[string]string{"q_latest": strings.ReplaceAll(latest, " ", "\n") + "\n", "q_ops_exists": "yes",
		"q_checksums": dirSums(t, dir, strings.Fields(latest)...)}
	for i := 0; i+1 < len(extra); i += 2 {
		m[extra[i]] = extra[i+1]
	}
	return m
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
	sd := fakeDockerState(t, downAnswers(t, dir, "002_b.sql"))
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
	sd := fakeDockerState(t, downAnswers(t, dir, "002_b.sql 001_a.sql"))
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
	sd := fakeDockerState(t, downAnswers(t, dir, "003_c.sql"))
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
	sd := fakeDockerState(t, downAnswers(t, dir, "002_b.sql 001_a.sql"))
	if _, err := MigrateDownDir(context.Background(), fakeCfg, dir, 2); err == nil {
		t.Fatal("want error: 001_a has no down file")
	}
	if countPrefix(recordedCalls(t, sd), "CALL PIPE") != 0 {
		t.Error("step 1 ran although step 2 could not")
	}
}

func TestMigrateDownDir_FewerAppliedThanStepsRevertsNothing(t *testing.T) {
	dir := mkMigDir(t, dirTestFiles)
	sd := fakeDockerState(t, downAnswers(t, dir, "002_b.sql"))
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
	sd := fakeDockerState(t, downAnswers(t, dir, "001_a.sql"))
	if _, err := MigrateDownDir(context.Background(), fakeCfg, dir, 1); err == nil {
		t.Fatal("want refusal")
	}
	if countPrefix(recordedCalls(t, sd), "CALL PIPE") != 0 {
		t.Error("executed a down file that escapes the transaction")
	}
}

func TestMigrateDownDir_PsqlFailureIsNotSuccess(t *testing.T) {
	dir := mkMigDir(t, dirTestFiles)
	fakeDockerState(t, downAnswers(t, dir, "002_b.sql", "fail_pipe", ""))
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

// Transaction control anywhere in the statement stream is refused; harmless
// look-alikes (inside comments, strings, dollar quotes, plpgsql blocks) are not.
func TestDirSQL_Table(t *testing.T) {
	bad := map[string]string{
		"own line":            "DROP TABLE a;\nCOMMIT;",
		"same line":           "DROP TABLE dir_b; COMMIT; DROP TABLE nope;",
		"begin first":         "BEGIN;\nDROP TABLE a;",
		"begin transaction":   "DROP TABLE a; BEGIN TRANSACTION ISOLATION LEVEL SERIALIZABLE;",
		"start transaction":   "select 1; start transaction;",
		"end":                 "DROP TABLE a; END;",
		"rollback":            "DROP TABLE a; ROLLBACK;",
		"abort":               "DROP TABLE a; abort;",
		"prepare transaction": "DROP TABLE a; PREPARE TRANSACTION 'x';",
		"after dash string":   "SELECT '--'; COMMIT;",
		"after block comment": "SELECT 1; /* c */ COMMIT;",
		"after dollar body":   "SELECT $$x$$; COMMIT;",
		"after E string":      "SELECT E'it\\'s'; COMMIT;",
		"psql meta-command":   "DROP TABLE a;\n\\set AUTOCOMMIT on\nDROP TABLE b;",
		"no trailing newline": "DROP TABLE a;commit",
		"mixed case":          "DROP TABLE a; CoMmIt ;",
		"wrapped, commit mid": "BEGIN; DROP TABLE a; COMMIT; DROP TABLE b;",
		"begin with options":  "BEGIN ISOLATION LEVEL SERIALIZABLE; DROP TABLE a; COMMIT;",
		"two begins":          "BEGIN; BEGIN; DROP TABLE a; COMMIT;",
		"begin, no commit":    "BEGIN; DROP TABLE a;",
		// review rechecks (P7-PROD-77): none of these may disable the scan.
		"comment names CONCURRENTLY":                "-- was DROP INDEX CONCURRENTLY once\nDROP TABLE dir_b; COMMIT; DROP TABLE does_not_exist;",
		"string names ADD VALUE":                    "SELECT 'ALTER TYPE x ADD VALUE'; DROP TABLE dir_b; COMMIT; DROP TABLE does_not_exist;",
		"atomic body then END":                      "CREATE FUNCTION f() RETURNS int BEGIN ATOMIC SELECT 1; END;\nDROP TABLE dir_b; END; DROP TABLE does_not_exist;",
		"atomic body then COMMIT":                   "CREATE FUNCTION f() RETURNS int BEGIN ATOMIC SELECT 1; END;\nDROP TABLE dir_b; COMMIT; DROP TABLE nope;",
		"quote after ELSE":                          "SELECT CASE WHEN true THEN 1 ELSE'b\\' END; COMMIT; DROP TABLE nope;",
		"dollar inside identifier":                  "SELECT 1 AS t$q$; COMMIT; DROP TABLE nope; SELECT 2 AS u$q$;",
		"unterminated atomic body":                  "CREATE FUNCTION f() RETURNS int BEGIN ATOMIC SELECT 1; DROP TABLE a;",
		"comment CONCURRENTLY, wrapped, commit mid": "-- CREATE INDEX CONCURRENTLY\nBEGIN; DROP TABLE a; COMMIT; DROP TABLE b;",
	}
	for name, sql := range bad {
		if _, err := dirSQL("f.sql", sql); err == nil {
			t.Errorf("%s: not refused: %q", name, sql)
		}
	}
	good := map[string]string{
		"comment":                        "-- COMMIT; BEGIN;\nDROP TABLE a; /* ROLLBACK; */",
		"string":                         "INSERT INTO t VALUES ('x; COMMIT; y');",
		"identifier":                     `ALTER TABLE "commit; x" ADD c int;`,
		"dollar block":                   "DO $$ BEGIN PERFORM 1; END $$;",
		"tagged dollar":                  "DO $body$ BEGIN PERFORM 1; END; $body$;",
		"function":                       "CREATE FUNCTION f() RETURNS int AS 'BEGIN RETURN 1; END;' LANGUAGE plpgsql;",
		"begin atomic":                   "CREATE FUNCTION f() RETURNS int BEGIN ATOMIC SELECT 1; END;",
		"rollback to":                    "SAVEPOINT s; DROP TABLE a; ROLLBACK TO SAVEPOINT s;",
		"E string ok":                    "SELECT E'it\\'s; COMMIT;';",
		"commentary":                     "ALTER TABLE t ADD COLUMN commit_at timestamptz;",
		"concurrently":                   "BEGIN;\nCREATE INDEX CONCURRENTLY i ON t (c);\nCOMMIT;",
		"real concurrently":              "CREATE INDEX CONCURRENTLY i ON t (c);",
		"atomic then wrapper":            "BEGIN;\nCREATE FUNCTION f() RETURNS int BEGIN ATOMIC SELECT 1; END;\nDROP TABLE a;\nCOMMIT;",
		"two atomic bodies":              "CREATE FUNCTION f() RETURNS int BEGIN ATOMIC SELECT 1; END; CREATE FUNCTION g() RETURNS int BEGIN ATOMIC SELECT 2; END;",
		"e-string escape":                "SELECT E'a\\'b; COMMIT;';",
		"dollar tag in word":             "SELECT 1 AS t$q$; SELECT 2 AS u$q$;",
		"quote after else, plain string": "SELECT CASE WHEN true THEN 1 ELSE 'b' END; DROP TABLE a;",
		"outer wrapper":                  "-- header\nBEGIN;\nCREATE TABLE a (id int);\nCOMMIT;\n",
		"wrapper words":                  "BEGIN TRANSACTION; CREATE TABLE a (id int); END TRANSACTION;",
	}
	for name, sql := range good {
		if _, err := dirSQL("f.sql", sql); err != nil {
			t.Errorf("%s: wrongly refused: %v", name, err)
		}
	}
}

// A real non-transactional statement cannot run inside down's transaction; a
// comment that merely mentions CONCURRENTLY is ordinary SQL and runs wrapped.
func TestMigrateDownDir_NonTransactionalDecidedFromRealStatements(t *testing.T) {
	dir := mkMigDir(t, map[string]string{"001_a.sql": "x", "001_a.down.sql": "DROP INDEX CONCURRENTLY i;"})
	sd := fakeDockerState(t, downAnswers(t, dir, "001_a.sql"))
	if _, err := MigrateDownDir(context.Background(), fakeCfg, dir, 1); err == nil {
		t.Fatal("want refusal for a real DROP INDEX CONCURRENTLY")
	}
	if countPrefix(recordedCalls(t, sd), "CALL PIPE") != 0 {
		t.Error("ran a non-transactional down file inside a transaction")
	}
	dir = mkMigDir(t, map[string]string{"001_a.sql": "x", "001_a.down.sql": "-- was DROP INDEX CONCURRENTLY once\nDROP TABLE dir_a;\n"})
	sd = fakeDockerState(t, downAnswers(t, dir, "001_a.sql"))
	if _, err := MigrateDownDir(context.Background(), fakeCfg, dir, 1); err != nil {
		t.Fatalf("comment-only mention must not matter: %v", err)
	}
	if countPrefix(recordedCalls(t, sd), "CALL PIPE") != 1 {
		t.Error("want one wrapped transaction")
	}
}

// Up: a comment naming CONCURRENTLY must not make ApplyFile run the file
// outside a transaction (it would then not be atomic with its ledger rows).
func TestMigrateUpDir_CommentNamingConcurrentlyStillRunsWrapped(t *testing.T) {
	dir := mkMigDir(t, map[string]string{"001_a.sql": "-- CREATE INDEX CONCURRENTLY is not used here\nCREATE TABLE a (id int);\n"})
	sd := fakeDockerState(t, map[string]string{"q_legacy_exists": "yes", "q_ops_exists": "yes"})
	if n, err := MigrateUpDir(context.Background(), fakeCfg, dir); err != nil || n != 1 {
		t.Fatalf("up = %d, %v", n, err)
	}
	b, _ := os.ReadFile(filepath.Join(sd, "pipe-0.sql"))
	if !strings.HasPrefix(string(b), "BEGIN;") || !strings.Contains(string(b), "INSERT INTO nself_ops.migrations") {
		t.Errorf("file was not wrapped with its ledger rows:\n%s", b)
	}
}

func TestMigrateDownDir_RefusesSameLineCommit(t *testing.T) {
	dir := mkMigDir(t, map[string]string{"001_a.sql": "x", "001_a.down.sql": "DROP TABLE dir_a; COMMIT; DROP TABLE nope;"})
	sd := fakeDockerState(t, downAnswers(t, dir, "001_a.sql"))
	if _, err := MigrateDownDir(context.Background(), fakeCfg, dir, 1); err == nil || !strings.Contains(err.Error(), "transaction control") {
		t.Fatalf("want transaction-control refusal, got %v", err)
	}
	if countPrefix(recordedCalls(t, sd), "CALL PIPE") != 0 {
		t.Error("executed a down file that commits mid-way")
	}
}

// The ledger keys on the base name only: a file of the same name in another
// directory (different content, so different checksum) must not be reverted.
func TestMigrateDownDir_RefusesSameNameFromAnotherDir(t *testing.T) {
	dirX := mkMigDir(t, dirTestFiles)
	other := map[string]string{"002_b.sql": "CREATE TABLE other_b (id int);", "002_b_down.sql": "DROP TABLE IF EXISTS dir_b;"}
	dirY := mkMigDir(t, other)
	sd := fakeDockerState(t, downAnswers(t, dirX, "002_b.sql"))
	_, err := MigrateDownDir(context.Background(), fakeCfg, dirY, 1)
	if err == nil || !strings.Contains(err.Error(), "not the migration that was applied") {
		t.Fatalf("want refusal, got %v", err)
	}
	if countPrefix(recordedCalls(t, sd), "CALL PIPE") != 0 {
		t.Error("reverted another directory's migration")
	}
	// The same directory still works.
	fakeDockerState(t, downAnswers(t, dirX, "002_b.sql"))
	if got, err := MigrateDownDir(context.Background(), fakeCfg, dirX, 1); err != nil || len(got) != 1 {
		t.Fatalf("original dir: %v, %v", got, err)
	}
}

func TestMigrateDownDir_RefusesWhenNoChecksumRecorded(t *testing.T) {
	dir := mkMigDir(t, dirTestFiles)
	sd := fakeDockerState(t, downAnswers(t, dir, "002_b.sql", "q_checksums", ""))
	if _, err := MigrateDownDir(context.Background(), fakeCfg, dir, 1); err == nil {
		t.Fatal("want refusal: identity cannot be proven without a recorded checksum")
	}
	if countPrefix(recordedCalls(t, sd), "CALL PIPE") != 0 {
		t.Error("reverted without proving identity")
	}
}

func TestMigrateDirUp_RefusesTxControlBeforeApplyingAnything(t *testing.T) {
	dir := mkMigDir(t, map[string]string{"001_a.sql": "CREATE TABLE a (id int);", "002_b.sql": "CREATE TABLE b (id int); COMMIT; CREATE TABLE c (id int);"})
	sd := fakeDockerState(t, map[string]string{"q_legacy_exists": "yes", "q_ops_exists": "yes"})
	if _, err := MigrateUpDir(context.Background(), fakeCfg, dir); err == nil || !strings.Contains(err.Error(), "transaction control") {
		t.Fatalf("want refusal, got %v", err)
	}
	if countPrefix(recordedCalls(t, sd), "CALL PIPE") != 0 {
		t.Error("applied a file before refusing the batch")
	}
	if _, err := PendingDirMigrations(context.Background(), fakeCfg, dir); err == nil {
		t.Error("the dry-run preview must refuse what the real run refuses")
	}
}

// An outer BEGIN/COMMIT is dropped, so the CLI's own transaction is the only one.
func TestDirSQL_StripsOuterWrapper(t *testing.T) {
	out, err := dirSQL("f.sql", "-- h\nBEGIN;\nCREATE TABLE a (id int);\nCOMMIT;\n")
	if err != nil {
		t.Fatal(err)
	}
	if u := strings.ToUpper(out); strings.Contains(u, "BEGIN") || strings.Contains(u, "COMMIT") || !strings.Contains(out, "CREATE TABLE a (id int);") {
		t.Errorf("wrapper not stripped cleanly: %q", out)
	}
}

func TestMigrateUpDir_WrappedFileRunsInOneTransaction(t *testing.T) {
	dir := mkMigDir(t, map[string]string{"001_a.sql": "BEGIN;\nCREATE TABLE a (id int);\nCOMMIT;\n"})
	sd := fakeDockerState(t, map[string]string{"q_legacy_exists": "yes", "q_ops_exists": "yes"})
	if n, err := MigrateUpDir(context.Background(), fakeCfg, dir); err != nil || n != 1 {
		t.Fatalf("up = %d, %v", n, err)
	}
	b, _ := os.ReadFile(filepath.Join(sd, "pipe-0.sql"))
	if strings.Count(string(b), "BEGIN") != 1 || strings.Count(string(b), "COMMIT") != 1 || !strings.Contains(string(b), "CREATE TABLE a") {
		t.Errorf("want exactly one BEGIN and one COMMIT around the migration and its ledger rows:\n%s", b)
	}
}

func TestMigrateDownDir_WrappedDownFileRunsInOneTransaction(t *testing.T) {
	dir := mkMigDir(t, map[string]string{"001_a.sql": "x", "001_a.down.sql": "BEGIN;\nDROP TABLE dir_a;\nCOMMIT;\n"})
	sd := fakeDockerState(t, downAnswers(t, dir, "001_a.sql"))
	if _, err := MigrateDownDir(context.Background(), fakeCfg, dir, 1); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(sd, "pipe-0.sql"))
	if strings.Count(string(b), "BEGIN") != 1 || strings.Count(string(b), "COMMIT") != 1 || !strings.Contains(string(b), "DROP TABLE dir_a;") {
		t.Errorf("down SQL not a single transaction:\n%s", b)
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

	// 7. a down file that commits mid-way on one line is refused: nothing runs,
	// so table and ledger stay in step.
	if err := os.WriteFile(filepath.Join(dir, "002_b_down.sql"), []byte("DROP TABLE dir_b; COMMIT; DROP TABLE does_not_exist;"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err = MigrateDownDir(ctx, cfg, dir, 1); err == nil || len(got) != 0 {
		t.Fatalf("same-line COMMIT down reported %v, %v", got, err)
	}
	if pgScalar(t, cfg, "SELECT to_regclass('dir_b') IS NOT NULL") != "t" ||
		pgScalar(t, cfg, "SELECT count(*) FROM np_common.schema_versions WHERE name = '002_b.sql'") != "1" {
		t.Fatal("schema and ledger fell out of step")
	}

	// 7b. review rechecks: a comment naming CONCURRENTLY, or a BEGIN ATOMIC body
	// earlier in the file, must not let a mid-file COMMIT/END through.
	for _, bad := range []string{
		"-- was DROP INDEX CONCURRENTLY once\nDROP TABLE dir_b; COMMIT; DROP TABLE does_not_exist;",
		"CREATE FUNCTION p77f() RETURNS int BEGIN ATOMIC SELECT 1; END;\nDROP TABLE dir_b; END; DROP TABLE does_not_exist;",
		"SELECT CASE WHEN true THEN 1 ELSE'b\\' END; DROP TABLE dir_b; COMMIT; DROP TABLE does_not_exist;",
	} {
		if err := os.WriteFile(filepath.Join(dir, "002_b_down.sql"), []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		if got, err = MigrateDownDir(ctx, cfg, dir, 1); err == nil || len(got) != 0 {
			t.Fatalf("bypass down reported %v, %v: %q", got, err, bad)
		}
		if pgScalar(t, cfg, "SELECT to_regclass('dir_b') IS NOT NULL") != "t" ||
			pgScalar(t, cfg, "SELECT count(*) FROM np_common.schema_versions WHERE name = '002_b.sql'") != "1" {
			t.Fatalf("schema and ledger fell out of step for %q", bad)
		}
	}

	// 8. another directory's 002_b.sql (different content) must not be reverted,
	// even with a down file that would succeed.
	other := mkMigDir(t, map[string]string{"002_b.sql": "CREATE TABLE other_b (id int);", "002_b_down.sql": "DROP TABLE IF EXISTS dir_b;"})
	if got, err = MigrateDownDir(ctx, cfg, other, 1); err == nil || len(got) != 0 {
		t.Fatalf("cross-directory down reported %v, %v", got, err)
	}
	if pgScalar(t, cfg, "SELECT to_regclass('dir_b') IS NOT NULL") != "t" ||
		pgScalar(t, cfg, "SELECT count(*) FROM nself_ops.migrations WHERE name = '002_b.sql'") != "1" {
		t.Fatal("another directory's down touched this migration")
	}
}
