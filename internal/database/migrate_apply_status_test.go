package database

// Purpose: tests for D-0378 (`db migrate status` is read-only) and for the
// ledger upgrade that moved into the up and down paths. A fake `docker`
// (migrate_dir_down_test.go) records every statement.
// Inputs: temp migration directories, canned ledger answers.
// Outputs: assertions over recorded statements, statuses and the warning text.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// swapStatusWarn captures statusWarnOut for the test.
func swapStatusWarn(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := statusWarnOut
	statusWarnOut = &buf
	t.Cleanup(func() { statusWarnOut = old })
	return &buf
}

// assertNoWrites fails unless every recorded call is a SELECT query.
func assertNoWrites(t *testing.T, calls []string) {
	t.Helper()
	if n := countPrefix(calls, "CALL EXEC") + countPrefix(calls, "CALL PIPE"); n != 0 {
		t.Fatalf("status wrote (%d exec/pipe): %v", n, calls)
	}
	for _, c := range calls {
		if !strings.HasPrefix(c, "CALL ") {
			continue // continuation line of a multi-line query in the fake's log
		}
		if !strings.HasPrefix(c, "CALL QUERY SELECT ") {
			t.Errorf("non-SELECT statement: %s", c)
		}
	}
}

// nestedDir builds hasura-style <ts>_<name>/up.sql directories.
func nestedDir(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(dir, n), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, n, "up.sql"), []byte("SELECT 1;"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestMigrateStatus_ExistingTablesIssuesQueriesOnly(t *testing.T) {
	dir := mkMigDir(t, dirTestFiles)
	warn := swapStatusWarn(t)
	sd := fakeDockerState(t, map[string]string{
		"q_legacy_exists": "yes", "q_ops_exists": "yes", "q_ops_legacy_count": "0",
		"q_applied": "001_a.sql|2026-10-04T10:00:00Z",
	})
	got, err := MigrateStatus(context.Background(), fakeCfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "001_a.sql" || !got[0].Applied || got[1].Name != "002_b.sql" || got[1].Applied {
		t.Fatalf("statuses = %+v", got)
	}
	assertNoWrites(t, recordedCalls(t, sd))
	if warn.Len() != 0 {
		t.Errorf("clean ledger warned: %q", warn.String())
	}
}

func TestMigrateStatus_MissingTablesCreatesNothing(t *testing.T) {
	dir := mkMigDir(t, dirTestFiles)
	warn := swapStatusWarn(t)
	sd := fakeDockerState(t, map[string]string{"q_legacy_exists": "no", "q_ops_exists": "no"})
	got, err := MigrateStatus(context.Background(), fakeCfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Applied || got[1].Applied {
		t.Fatalf("want both files pending, got %+v", got)
	}
	calls := recordedCalls(t, sd)
	assertNoWrites(t, calls)
	for _, c := range calls {
		if strings.Contains(c, "CREATE") {
			t.Errorf("status issued a CREATE: %s", c)
		}
	}
	if warn.Len() != 0 {
		t.Errorf("missing ledger warned: %q", warn.String())
	}
}

func TestMigrateStatus_LegacyUpRowResolvedInMemoryWithWarning(t *testing.T) {
	dir := nestedDir(t, "001_x", "002_y")
	warn := swapStatusWarn(t)
	sd := fakeDockerState(t, map[string]string{
		"q_legacy_exists": "yes", "q_ops_exists": "yes", "q_ops_legacy_count": "0",
		"q_applied": "up.sql|2026-10-04T10:00:00Z", "q_up_count": "1", "q_ops_ids": "001\n",
	})
	got, err := MigrateStatus(context.Background(), fakeCfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "001_x" || !got[0].Applied || got[1].Name != "002_y" || got[1].Applied {
		t.Fatalf("statuses = %+v, want 001_x applied via the up.sql row and 002_y pending", got)
	}
	assertNoWrites(t, recordedCalls(t, sd))
	if !strings.Contains(warn.String(), "legacy rows") || !strings.Contains(warn.String(), "nself db migrate up") {
		t.Errorf("warning = %q", warn.String())
	}
	if strings.Count(warn.String(), "\n") != 1 {
		t.Errorf("want one warning line, got %q", warn.String())
	}
}

func TestMigrateStatus_LegacyOpsIDsWarn(t *testing.T) {
	dir := mkMigDir(t, dirTestFiles)
	warn := swapStatusWarn(t)
	sd := fakeDockerState(t, map[string]string{
		"q_legacy_exists": "yes", "q_ops_exists": "yes", "q_ops_legacy_count": "2",
		"q_applied": "001_a.sql|2026-10-04T10:00:00Z",
	})
	if _, err := MigrateStatus(context.Background(), fakeCfg, dir); err != nil {
		t.Fatal(err)
	}
	assertNoWrites(t, recordedCalls(t, sd))
	if !strings.Contains(warn.String(), "legacy rows") {
		t.Errorf("no warning for legacy ops ids: %q", warn.String())
	}
}

// upToDateAnswers is a ledger that lists both dirTestFiles as applied.
func upToDateAnswers(t *testing.T, dir string, extra map[string]string) map[string]string {
	t.Helper()
	m := map[string]string{
		"q_legacy_exists": "yes", "q_ops_exists": "yes", "q_ops_legacy_count": "0",
		"q_applied":   appliedAnswer([]string{"001_a.sql", "002_b.sql"}),
		"q_checksums": dirSums(t, dir, "001_a.sql", "002_b.sql"),
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestMigrateUpDir_LegacyLedgerPipesBackfill(t *testing.T) {
	dir := mkMigDir(t, dirTestFiles)
	sd := fakeDockerState(t, upToDateAnswers(t, dir, map[string]string{"q_ops_legacy_count": "3"}))
	if n, err := MigrateUpDir(context.Background(), fakeCfg, dir); err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	b, err := os.ReadFile(filepath.Join(sd, "pipe-0.sql"))
	if err != nil {
		t.Fatalf("no backfill was piped: %v", err)
	}
	if strings.TrimSpace(string(b)) != strings.TrimSpace(legacyOpsIDBackfillSQL) {
		t.Errorf("piped SQL is not the backfill:\n%s", b)
	}
}

func TestMigrateUpDir_CleanLedgerPipesNoBackfill(t *testing.T) {
	dir := mkMigDir(t, dirTestFiles)
	sd := fakeDockerState(t, upToDateAnswers(t, dir, nil))
	if _, err := MigrateUpDir(context.Background(), fakeCfg, dir); err != nil {
		t.Fatal(err)
	}
	if n := countPrefix(recordedCalls(t, sd), "CALL PIPE"); n != 0 {
		t.Errorf("clean ledger piped %d batches", n)
	}
}

func TestMigrateDownDir_LegacyLedgerUpgradedBeforeRevert(t *testing.T) {
	dir := mkMigDir(t, dirTestFiles)
	sd := fakeDockerState(t, downAnswers(t, dir, "002_b.sql", "q_ops_legacy_count", "1"))
	if _, err := MigrateDownDir(context.Background(), fakeCfg, dir, 1); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(filepath.Join(sd, "pipe-0.sql"))
	if !strings.Contains(string(first), "UPDATE nself_ops.migrations m") {
		t.Errorf("first pipe is not the backfill:\n%s", first)
	}
	if second, _ := os.ReadFile(filepath.Join(sd, "pipe-1.sql")); !strings.Contains(string(second), "DROP TABLE dir_b;") {
		t.Errorf("second pipe is not the revert:\n%s", second)
	}
}

func TestLedgerNeedsUpgrade_SharedPredicate(t *testing.T) {
	if legacyOpsIDBackfillWhere == "" {
		t.Fatal("empty shared predicate")
	}
	for name, q := range map[string]string{"UPDATE": legacyOpsIDBackfillSQL, "count": legacyOpsIDCountSQL} {
		if !strings.Contains(q, legacyOpsIDBackfillWhere) {
			t.Errorf("%s query lacks the shared predicate:\n%s", name, q)
		}
	}
	if !strings.HasPrefix(legacyOpsIDBackfillSQL, "UPDATE ") || !strings.HasPrefix(legacyOpsIDCountSQL, "SELECT count(*)") {
		t.Error("unexpected statement shapes")
	}
}

func TestLedgerNeedsUpgrade_MissingTablesIsFalse(t *testing.T) {
	sd := fakeDockerState(t, map[string]string{"q_legacy_exists": "no", "q_ops_exists": "no"})
	need, err := ledgerNeedsUpgrade(context.Background(), fakeCfg, []string{"/x/001_x/up.sql"})
	if err != nil || need {
		t.Fatalf("need=%v err=%v, want false, nil", need, err)
	}
	assertNoWrites(t, recordedCalls(t, sd))
}

func TestLedgerNeedsUpgrade_NonIntegerCountIsError(t *testing.T) {
	fakeDockerState(t, map[string]string{"q_ops_exists": "yes", "q_ops_legacy_count": "boom"})
	if _, err := ledgerNeedsUpgrade(context.Background(), fakeCfg, nil); err == nil {
		t.Fatal("want an error for a non-integer count")
	}
}

// pipeFile reads the n-th SQL batch the fake docker received.
func pipeFile(t *testing.T, sd string, n int) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(sd, fmt.Sprintf("pipe-%d.sql", n)))
	if err != nil {
		t.Fatalf("pipe-%d.sql: %v", n, err)
	}
	return string(b)
}

func TestMigrateUpDir_NestedDirLegacyUpRowPipesRename(t *testing.T) {
	dir := nestedDir(t, "001_x")
	sd := fakeDockerState(t, map[string]string{
		"q_legacy_exists": "yes", "q_ops_exists": "yes", "q_ops_legacy_count": "0",
		"q_applied": "up.sql|2026-10-04T10:00:00Z", "q_up_count": "1", "q_ops_ids": "001\n",
	})
	if _, err := MigrateUpDir(context.Background(), fakeCfg, dir); err != nil {
		t.Fatal(err)
	}
	if got := pipeFile(t, sd, 0); strings.TrimSpace(got) != strings.TrimSpace(legacyOpsIDBackfillSQL) {
		t.Errorf("pipe-0 is not the backfill:\n%s", got)
	}
	if got := pipeFile(t, sd, 1); got != legacyNestedRenameSQL("001_x") {
		t.Errorf("pipe-1 is not the nested rename:\n%s", got)
	}
}

func TestMigrateDownDir_NestedLegacyUpRowUpgradedBeforeLedgerRead(t *testing.T) {
	dir := mkMigDir(t, dirTestFiles)
	if err := os.MkdirAll(filepath.Join(dir, "000_n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "000_n", "up.sql"), []byte("SELECT 1;"), 0o600); err != nil {
		t.Fatal(err)
	}
	sd := fakeDockerState(t, downAnswers(t, dir, "002_b.sql", "q_up_count", "1"))
	if _, err := MigrateDownDir(context.Background(), fakeCfg, dir, 1); err != nil {
		t.Fatal(err)
	}
	if got := pipeFile(t, sd, 1); got != legacyNestedRenameSQL("000_n") {
		t.Errorf("pipe-1 is not the nested rename:\n%s", got)
	}
	if got := pipeFile(t, sd, 2); !strings.Contains(got, "DROP TABLE dir_b;") {
		t.Errorf("pipe-2 is not the revert:\n%s", got)
	}
	calls, lastPipeBeforeRead, read := recordedCalls(t, sd), -1, -1
	for i, c := range calls {
		if strings.HasPrefix(c, "CALL QUERY SELECT name FROM np_common.schema_versions ORDER BY applied_at DESC") {
			read = i
			break
		}
		if strings.HasPrefix(c, "CALL PIPE") {
			lastPipeBeforeRead = i
		}
	}
	if read < 0 || lastPipeBeforeRead < 0 {
		t.Errorf("upgrade did not run before the ledger read: %v", calls)
	}
}

func TestMigrateUpDir_FlatDirLegacyUpRowPipesNoUpgrade(t *testing.T) {
	dir := mkMigDir(t, dirTestFiles)
	sd := fakeDockerState(t, upToDateAnswers(t, dir, map[string]string{
		"q_applied": appliedAnswer([]string{"001_a.sql", "002_b.sql", "up.sql"}), "q_up_count": "1",
	}))
	if _, err := MigrateUpDir(context.Background(), fakeCfg, dir); err != nil {
		t.Fatal(err)
	}
	if n := countPrefix(recordedCalls(t, sd), "CALL PIPE"); n != 0 {
		t.Errorf("flat dir with no nested migration piped %d upgrade batches", n)
	}
}

func TestMigrateUpDir_NoopRerunExecBudget(t *testing.T) {
	dir := mkMigDir(t, dirTestFiles)
	fakeDockerState(t, upToDateAnswers(t, dir, nil))
	execs, restore := countExecs(t)
	_, err := MigrateUpDir(context.Background(), fakeCfg, dir)
	restore()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("execs=%d", execs())
	// ensure (1) + legacy ops-id count (1) + two ledger reads; no to_regclass probes.
	if execs() > 4 {
		t.Errorf("%d execs on a flat no-op rerun, want <= 4", execs())
	}
}
