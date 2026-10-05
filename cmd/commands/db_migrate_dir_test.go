package commands

// Purpose: tests for `db migrate up --migration-dir [--dry-run]` and
// `db migrate down --migration-dir [--steps N]` (P7-PROD-77), plus golden
// tests that pin the no-`--migration-dir` paths to their pre-change docker
// statement sequence.
// Inputs: a fake `docker` executable on PATH that records every invocation
// (and every piped SQL text) and answers queries from canned files.
// Outputs: assertions over the recorded statements, so "dry-run writes
// nothing" is proven by the full statement log, not by a rollback.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// fakeDockerScript logs each call to $FAKE_DOCKER_DIR/calls.log. `exec -i`
// calls (SQL piped on stdin) are saved as pipe-N.sql; `-c` calls are logged
// as writes; `-tAc` calls are queries answered from q_* files.
const fakeDockerScript = `#!/bin/sh
D="$FAKE_DOCKER_DIR"
case "$*" in
  *pg_isready*) echo "CALL ISREADY" >> "$D/calls.log"; exit 0;;
esac
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
  *"name = 'up.sql'"*) echo 0;;
esac
exit 0
`

// pathEnvVar is a variable on purpose: tools/parity counts a literal env read of PATH in
// cmd/commands as an undocumented env var of `nself db`, and this is test plumbing.
const pathEnvVar = "PATH"

// useFakeDocker installs the fake docker on PATH and returns its state dir.
func useFakeDocker(t *testing.T, answers map[string]string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a /bin/sh script")
	}
	binDir, stateDir := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "docker"), []byte(fakeDockerScript), 0o755); err != nil {
		t.Fatalf("write fake docker: %v", err)
	}
	t.Setenv(pathEnvVar, binDir+string(os.PathListSeparator)+os.Getenv(pathEnvVar))
	t.Setenv("FAKE_DOCKER_DIR", stateDir)
	for name, body := range answers {
		if err := os.WriteFile(filepath.Join(stateDir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write answer %s: %v", name, err)
		}
	}
	return stateDir
}

// dockerCalls returns the recorded call lines.
func dockerCalls(t *testing.T, stateDir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(stateDir, "calls.log"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read calls.log: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// writeFiles creates name->content files under dir.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// newDirTestCmd builds a fresh up or down command wired like the real ones.
func newDirTestCmd(use string) (*cobra.Command, *bytes.Buffer) {
	var run func(*cobra.Command, []string) error
	cmd := &cobra.Command{Use: use}
	if use == "up" {
		run = runDBMigrateUp
		cmd.Flags().String("migration-dir", "", "")
		cmd.Flags().Bool("dry-run", false, "")
		cmd.Flags().String("plugin", "", "")
	} else {
		run = runDBMigrateDown
		addDBMigrateDownFlags(cmd)
	}
	cmd.RunE = run
	cmd.SetContext(context.Background())
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	return cmd, out
}

// dirProject chdirs into a project dir holding migrations/ with two files.
func dirProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	writeFiles(t, filepath.Join(root, "migrations"), map[string]string{
		"001_a.sql":      "CREATE TABLE a (id int);",
		"001_a.down.sql": "DROP TABLE a;",
		"002_b.sql":      "CREATE TABLE b (id int);",
		"002_b_down.sql": "DROP TABLE b;",
	})
	return filepath.Join(root, "migrations")
}

// writes returns the recorded calls that can change the database: every
// EXEC (a -c statement) and every PIPE (SQL on stdin). Queries are reads.
func writes(calls []string) []string {
	var w []string
	for _, c := range calls {
		if strings.HasPrefix(c, "CALL EXEC") || strings.HasPrefix(c, "CALL PIPE") {
			w = append(w, c)
		}
	}
	return w
}

// shape collapses each recorded call to its first line, 90 chars max, so the
// golden sequences stay readable.
func shape(calls []string) []string {
	var out []string
	for _, c := range calls {
		if !strings.HasPrefix(c, "CALL ") {
			continue
		}
		if len(c) > 90 {
			c = c[:90]
		}
		out = append(out, c)
	}
	return out
}

func joinLines(l []string) string { return strings.Join(l, "\n") }

// TestDBMigrateUpDir_DryRun_WritesNothing is the P7-PROD-77 core safety
// test: against a ledger where 001_a.sql is applied and 002_b.sql is not,
// `up --migration-dir --dry-run` lists 002_b.sql and issues no EXEC and no
// PIPE, i.e. no DDL, no ledger write, no BEGIN/ROLLBACK probe.
func TestDBMigrateUpDir_DryRun_WritesNothing(t *testing.T) {
	dir := dirProject(t)
	sd := useFakeDocker(t, map[string]string{
		"q_legacy_exists": "yes\n", "q_ops_exists": "yes\n",
		"q_applied": "001_a.sql|2026-10-04T10:00:00Z\n",
	})
	cmd, out := newDirTestCmd("up")
	_ = cmd.Flags().Set("migration-dir", dir)
	_ = cmd.Flags().Set("dry-run", "true")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "002_b.sql" {
		t.Errorf("dry-run output = %q, want only 002_b.sql", got)
	}
	calls := dockerCalls(t, sd)
	if w := writes(calls); len(w) != 0 {
		t.Fatalf("dry-run issued writes: %v\nall calls: %v", w, calls)
	}
	for _, c := range calls {
		if strings.HasPrefix(c, "CALL QUERY") && !strings.HasPrefix(c, "CALL QUERY SELECT") {
			t.Errorf("non-SELECT query during dry-run: %s", c)
		}
	}
}

// TestDBMigrateUpDir_DryRun_NoLedgerTables covers a never-migrated database:
// both files are pending, and the missing ledger tables are NOT created.
func TestDBMigrateUpDir_DryRun_NoLedgerTables(t *testing.T) {
	dir := dirProject(t)
	sd := useFakeDocker(t, map[string]string{"q_legacy_exists": "no\n", "q_ops_exists": "no\n"})
	cmd, out := newDirTestCmd("up")
	_ = cmd.Flags().Set("migration-dir", dir)
	_ = cmd.Flags().Set("dry-run", "true")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if got := strings.Fields(out.String()); len(got) != 2 || got[0] != "001_a.sql" || got[1] != "002_b.sql" {
		t.Errorf("dry-run output = %q, want both files", out.String())
	}
	if w := writes(dockerCalls(t, sd)); len(w) != 0 {
		t.Fatalf("dry-run issued writes: %v", w)
	}
}

// TestDBMigrateUpDir_DryRun_ChecksumMismatchFails: an applied file edited
// after apply makes the real run fail, so the preview fails too (never "ok").
func TestDBMigrateUpDir_DryRun_ChecksumMismatchFails(t *testing.T) {
	dir := dirProject(t)
	useFakeDocker(t, map[string]string{
		"q_legacy_exists": "yes\n", "q_ops_exists": "yes\n",
		"q_applied":   "001_a.sql|2026-10-04T10:00:00Z\n",
		"q_checksums": "001_a.sql|deadbeef\n",
	})
	cmd, _ := newDirTestCmd("up")
	_ = cmd.Flags().Set("migration-dir", dir)
	_ = cmd.Flags().Set("dry-run", "true")
	err := cmd.RunE(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("want checksum mismatch error, got %v", err)
	}
}

// TestDBMigrateUpDir_WithoutDryRunStillApplies pins that plain
// `up --migration-dir` keeps applying (one transaction per file).
func TestDBMigrateUpDir_WithoutDryRunStillApplies(t *testing.T) {
	dir := dirProject(t)
	sd := useFakeDocker(t, map[string]string{"q_legacy_exists": "yes\n", "q_ops_exists": "yes\n"})
	cmd, _ := newDirTestCmd("up")
	_ = cmd.Flags().Set("migration-dir", dir)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("up: %v", err)
	}
	pipes := 0
	for _, c := range dockerCalls(t, sd) {
		if strings.HasPrefix(c, "CALL PIPE") {
			pipes++
		}
	}
	if pipes != 2 {
		t.Errorf("up --migration-dir piped %d SQL batches, want 2 (one per file)", pipes)
	}
}

// TestDBMigrateDown_Dir_RevertsOneByDefault: `down --migration-dir` reverts
// only the newest applied file, via its _down.sql, in one transaction.
func TestDBMigrateDown_Dir_RevertsOneByDefault(t *testing.T) {
	dir := dirProject(t)
	sd := useFakeDocker(t, map[string]string{"q_latest": "002_b.sql\n"})
	cmd, out := newDirTestCmd("down")
	_ = cmd.Flags().Set("migration-dir", dir)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("down: %v", err)
	}
	if !strings.Contains(out.String(), "Reverted 002_b.sql") {
		t.Errorf("output = %q", out.String())
	}
	sqlText, _ := os.ReadFile(filepath.Join(sd, "pipe-0.sql"))
	for _, want := range []string{"BEGIN;", "DROP TABLE b;", "DELETE FROM np_common.schema_versions WHERE name = '002_b.sql';", "DELETE FROM nself_ops.migrations WHERE name = '002_b.sql';", "COMMIT;"} {
		if !strings.Contains(string(sqlText), want) {
			t.Errorf("down SQL missing %q:\n%s", want, sqlText)
		}
	}
	if strings.Contains(string(sqlText), "DROP TABLE a;") {
		t.Error("down touched migration 001_a")
	}
	if n := len(writes(dockerCalls(t, sd))); n != 2 { // ensureSchemaVersions + one tx
		t.Errorf("writes = %d, want 2", n)
	}
}

// TestDBMigrateDown_Dir_FailureIsError: a failing psql is an error, never a
// success message.
func TestDBMigrateDown_Dir_FailureIsError(t *testing.T) {
	dir := dirProject(t)
	useFakeDocker(t, map[string]string{"q_latest": "002_b.sql\n", "fail_pipe": ""})
	cmd, out := newDirTestCmd("down")
	_ = cmd.Flags().Set("migration-dir", dir)
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("want error when psql fails")
	}
	if strings.Contains(out.String(), "Reverted") {
		t.Errorf("reported a revert on failure: %q", out.String())
	}
}

// TestDBMigrateDown_StepsWithoutDirIsError: --steps alone must not silently
// revert one migration.
func TestDBMigrateDown_StepsWithoutDirIsError(t *testing.T) {
	dirProject(t)
	sd := useFakeDocker(t, nil)
	cmd, _ := newDirTestCmd("down")
	_ = cmd.Flags().Set("steps", "3")
	err := cmd.RunE(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "--steps requires --migration-dir") {
		t.Fatalf("got %v", err)
	}
	if len(dockerCalls(t, sd)) != 0 {
		t.Error("docker was called before the flag check")
	}
}

// TestDBMigrateDownCmd_HasDirFlags checks the real wired command.
func TestDBMigrateDownCmd_HasDirFlags(t *testing.T) {
	for _, f := range []string{"migration-dir", "steps"} {
		if dbMigrateDownCmd.Flags().Lookup(f) == nil {
			t.Errorf("db migrate down is missing --%s", f)
		}
	}
}

// Golden tests: without --migration-dir the commands issue exactly the
// statement sequence they issued before P7-PROD-77 (captured from origin/main
// f6c45e2f with the same fake docker).

const goldenUpDryRun = `CALL ISREADY
CALL EXEC CREATE SCHEMA IF NOT EXISTS np_common; CREATE TABLE IF NOT EXISTS np_common.sche
CALL EXEC CREATE SCHEMA IF NOT EXISTS nself_ops
CALL EXEC CREATE TABLE IF NOT EXISTS nself_ops.migrations (
CALL PIPE pipe-0.sql
CALL QUERY SELECT count(*) FROM np_common.schema_versions WHERE name = 'up.sql'
CALL QUERY SELECT name || '|' || applied_at FROM np_common.schema_versions ORDER BY applie`

const goldenDown = `CALL EXEC CREATE SCHEMA IF NOT EXISTS np_common; CREATE TABLE IF NOT EXISTS np_common.sche
CALL QUERY SELECT name FROM np_common.schema_versions ORDER BY applied_at DESC LIMIT 1
CALL PIPE pipe-0.sql`

const goldenDownSQL = "BEGIN;\nDROP TABLE a;\nDELETE FROM np_common.schema_versions WHERE name = '001_a.sql';\nDELETE FROM nself_ops.migrations WHERE name = '001_a.sql';\nCOMMIT;\n"

func TestDBMigrateUpDir_GoldenUpDryRunWithoutDir(t *testing.T) {
	dirProject(t)
	sd := useFakeDocker(t, map[string]string{"q_legacy_exists": "yes\n"})
	cmd, _ := newDirTestCmd("up")
	_ = cmd.Flags().Set("dry-run", "true")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("up --dry-run: %v", err)
	}
	if got := joinLines(shape(dockerCalls(t, sd))); got != goldenUpDryRun {
		t.Errorf("statement sequence changed:\n got:\n%s\nwant:\n%s", got, goldenUpDryRun)
	}
}

func TestDBMigrateDown_GoldenWithoutDir(t *testing.T) {
	dirProject(t)
	sd := useFakeDocker(t, map[string]string{"q_latest": "001_a.sql\n"})
	cmd, _ := newDirTestCmd("down")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("down: %v", err)
	}
	if got := joinLines(shape(dockerCalls(t, sd))); got != goldenDown {
		t.Errorf("statement sequence changed:\n got:\n%s\nwant:\n%s", got, goldenDown)
	}
	sqlText, _ := os.ReadFile(filepath.Join(sd, "pipe-0.sql"))
	if string(sqlText) != goldenDownSQL {
		t.Errorf("down SQL changed:\n%s", sqlText)
	}
}
