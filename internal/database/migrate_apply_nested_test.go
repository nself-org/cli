package database

// Purpose: tests for D-0613. A nested Hasura migration (<ts>_<name>/up.sql)
// must be keyed on its directory name (migrationKey), never on "up.sql", in
// ApplyFile, MigrateUpDir and PendingDirMigrations; flat files are unchanged.
// A fake `docker` (migrate_dir_down_test.go) records every statement.
// Inputs: temp migration directories with distinct bodies, canned ledger answers.
// Outputs: assertions over recorded statements, apply counts and pending lists.

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// nestedBodies maps dir name -> distinct up.sql body.
var nestedBodies = map[string]string{
	"001_x": "CREATE TABLE nest_x (id int);",
	"002_y": "CREATE TABLE nest_y (id int);",
}

// mkNested builds <dir>/<name>/up.sql for each name using nestedBodies.
func mkNested(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(dir, n), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, n, "up.sql"), []byte(nestedBodies[n]), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// nestedSums is the fake q_checksums answer keyed on the directory names.
func nestedSums(t *testing.T, names ...string) string {
	t.Helper()
	var lines []string
	for _, n := range names {
		sum, err := checksumBytes([]byte(nestedBodies[n]))
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, n+"|"+sum)
	}
	return strings.Join(lines, "\n")
}

// allPipes returns the text of every batch the fake docker received.
func allPipes(t *testing.T, sd string) []string {
	t.Helper()
	var out []string
	for i := 0; i < countPrefix(recordedCalls(t, sd), "CALL PIPE"); i++ {
		out = append(out, pipeFile(t, sd, i))
	}
	return out
}

func anyContains(batches []string, sub string) bool {
	for _, b := range batches {
		if strings.Contains(b, sub) {
			return true
		}
	}
	return false
}

// baseAnswers is a ledger with both tables present; extra overrides.
func baseAnswers(extra map[string]string) map[string]string {
	m := map[string]string{
		"q_legacy_exists": "yes", "q_ops_exists": "yes", "q_ops_legacy_count": "0",
		"q_applied": "", "q_up_count": "0", "q_count": "1",
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestMigrateUpDir_NestedDirLegacyUpRowPipesRename(t *testing.T) {
	dir := mkNested(t, "001_x")
	// readLedger runs after the upgrade, so the answers are the post-rename ledger.
	sd := fakeDockerState(t, baseAnswers(map[string]string{
		"q_applied": appliedAnswer([]string{"001_x"}), "q_checksums": nestedSums(t, "001_x"),
		"q_up_count": "1", "q_ops_ids": "001\n",
	}))
	n, err := MigrateUpDir(context.Background(), fakeCfg, dir)
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v, want 0, nil (the migration already ran)", n, err)
	}
	pipes := allPipes(t, sd)
	if len(pipes) != 2 {
		t.Fatalf("%d pipes, want 2 (backfill + rename): %v", len(pipes), pipes)
	}
	if strings.TrimSpace(pipes[0]) != strings.TrimSpace(legacyOpsIDBackfillSQL) {
		t.Errorf("pipe-0 is not the backfill:\n%s", pipes[0])
	}
	if pipes[1] != legacyNestedRenameSQL("001_x") {
		t.Errorf("pipe-1 is not the nested rename:\n%s", pipes[1])
	}
	if anyContains(pipes, "nest_x") {
		t.Error("an already-applied nested migration was run again")
	}
}

func TestMigrateUpDir_NestedDirsEachKeyedOnDirName(t *testing.T) {
	dir := mkNested(t, "001_x", "002_y")
	sd := fakeDockerState(t, baseAnswers(nil))
	n, err := MigrateUpDir(context.Background(), fakeCfg, dir)
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v, want 2, nil", n, err)
	}
	pipes := allPipes(t, sd)
	for _, want := range []string{"nest_x", "nest_y", "VALUES ('001_x')", "VALUES ('002_y')"} {
		if !anyContains(pipes, want) {
			t.Errorf("no batch contains %q: %v", want, pipes)
		}
	}
	if anyContains(pipes, "VALUES ('up.sql')") {
		t.Error("a ledger INSERT used the key up.sql")
	}
}

func TestMigrateUpDir_NestedRerunIsNoop(t *testing.T) {
	dir := mkNested(t, "001_x", "002_y")
	sd := fakeDockerState(t, baseAnswers(map[string]string{
		"q_applied": appliedAnswer([]string{"001_x", "002_y"}), "q_checksums": nestedSums(t, "001_x", "002_y"),
	}))
	n, err := MigrateUpDir(context.Background(), fakeCfg, dir)
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v, want 0, nil", n, err)
	}
	if p := countPrefix(recordedCalls(t, sd), "CALL PIPE"); p != 0 {
		t.Errorf("no-op rerun piped %d batches", p)
	}
}

func TestMigrateUpDir_NestedLegacyRowThenOnlyNewDirApplies(t *testing.T) {
	dir := mkNested(t, "001_x", "002_y")
	sd := fakeDockerState(t, baseAnswers(map[string]string{
		"q_applied": appliedAnswer([]string{"001_x"}), "q_checksums": nestedSums(t, "001_x"),
		"q_up_count": "1", "q_ops_ids": "001\n",
	}))
	n, err := MigrateUpDir(context.Background(), fakeCfg, dir)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v, want 1, nil", n, err)
	}
	pipes := allPipes(t, sd)
	if len(pipes) != 3 {
		t.Fatalf("%d pipes, want 3: %v", len(pipes), pipes)
	}
	if strings.TrimSpace(pipes[0]) != strings.TrimSpace(legacyOpsIDBackfillSQL) || pipes[1] != legacyNestedRenameSQL("001_x") {
		t.Errorf("upgrade batches wrong:\n%s\n%s", pipes[0], pipes[1])
	}
	if !strings.Contains(pipes[2], "nest_y") || !strings.Contains(pipes[2], "VALUES ('002_y')") {
		t.Errorf("pipe-2 is not 002_y's apply:\n%s", pipes[2])
	}
	if anyContains(pipes, "nest_x") {
		t.Error("001_x's body was run again")
	}
}

func TestPendingDirMigrations_NestedKeysOnDirName(t *testing.T) {
	cases := []struct {
		name    string
		answers map[string]string
	}{
		{"post-rename key", map[string]string{"q_applied": appliedAnswer([]string{"001_x"}), "q_checksums": nestedSums(t, "001_x")}},
		{"legacy up.sql row", map[string]string{"q_applied": appliedAnswer([]string{"up.sql"}), "q_ops_ids": "001\n"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := mkNested(t, "001_x", "002_y")
			sd := fakeDockerState(t, baseAnswers(c.answers))
			got, err := PendingDirMigrations(context.Background(), fakeCfg, dir)
			if err != nil {
				t.Fatal(err)
			}
			if want := []string{"002_y"}; !reflect.DeepEqual(got, want) {
				t.Errorf("pending = %v, want %v", got, want)
			}
			assertNoWrites(t, recordedCalls(t, sd))
		})
	}
}

func TestApplyFile_NestedUpSQLRefusedWhileLegacyRowExists(t *testing.T) {
	dir := mkNested(t, "001_x")
	sd := fakeDockerState(t, baseAnswers(map[string]string{"q_applied": appliedAnswer([]string{"up.sql"})}))
	skipped, err := ApplyFile(context.Background(), fakeCfg, filepath.Join(dir, "001_x", "up.sql"))
	if err == nil || !strings.Contains(err.Error(), "up.sql") || skipped {
		t.Fatalf("skipped=%v err=%v, want an error naming up.sql", skipped, err)
	}
	if !strings.Contains(err.Error(), "--migration-dir") {
		t.Errorf("error does not point at the upgrade command: %v", err)
	}
	if p := allPipes(t, sd); len(p) != 0 {
		t.Errorf("SQL ran despite the refusal: %v", p)
	}
}

func TestApplyFile_FlatRecordsFilenameAsBefore(t *testing.T) {
	dir := mkMigDir(t, map[string]string{"001_a.sql": "CREATE TABLE flat_a (id int);"})
	sd := fakeDockerState(t, baseAnswers(nil))
	skipped, err := ApplyFile(context.Background(), fakeCfg, filepath.Join(dir, "001_a.sql"))
	if err != nil || skipped {
		t.Fatalf("skipped=%v err=%v", skipped, err)
	}
	pipes := allPipes(t, sd)
	if len(pipes) != 1 {
		t.Fatalf("%d pipes, want 1: %v", len(pipes), pipes)
	}
	for _, want := range []string{
		"INSERT INTO np_common.schema_versions (name) VALUES ('001_a.sql');",
		"INSERT INTO nself_ops.migrations (id, name, checksum) VALUES ('001_a', '001_a.sql', '",
	} {
		if !strings.Contains(pipes[0], want) {
			t.Errorf("batch lacks %q:\n%s", want, pipes[0])
		}
	}
}

func TestApplyFile_SingleFileKeying(t *testing.T) {
	cases := []struct {
		name, file, applied, wantInsert string
		wantSkip                        bool
	}{
		{"flat file not refused by a legacy row", "001_a.sql", "up.sql", "VALUES ('001_a.sql');", false},
		{"nested file on a clean ledger records the dir key", "001_x/up.sql", "", "VALUES ('001_x');", false},
		{"nested file already applied under its key is skipped", "001_x/up.sql", "001_x", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := mkNested(t, "001_x")
			if err := os.WriteFile(filepath.Join(dir, "001_a.sql"), []byte("CREATE TABLE flat_a (id int);"), 0o600); err != nil {
				t.Fatal(err)
			}
			ans := map[string]string{}
			if tc.applied != "" {
				ans["q_applied"] = appliedAnswer([]string{tc.applied})
			}
			sd := fakeDockerState(t, baseAnswers(ans))
			skipped, err := ApplyFile(context.Background(), fakeCfg, filepath.Join(dir, tc.file))
			if err != nil || skipped != tc.wantSkip {
				t.Fatalf("skipped=%v err=%v, want skipped=%v", skipped, err, tc.wantSkip)
			}
			pipes := allPipes(t, sd)
			if tc.wantSkip {
				if len(pipes) != 0 {
					t.Errorf("skip ran SQL: %v", pipes)
				}
				return
			}
			if len(pipes) != 1 || !strings.Contains(pipes[0], tc.wantInsert) || strings.Contains(pipes[0], "VALUES ('up.sql')") {
				t.Errorf("want one batch with %q and no 'up.sql' row, got %v", tc.wantInsert, pipes)
			}
		})
	}
}
