package database

// Purpose: tests for D-0281: `-- nself:statement-timeout=` is honoured on the
// --migration-dir up and down paths (default stays 60s, lock_timeout stays 5s).
// Inputs: temp migration directories with and without the directive.
// Outputs: assertions over the piped transaction SQL.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const lock5 = "SET LOCAL lock_timeout = '5s';"

func stmtSet(v string) string { return "SET LOCAL statement_timeout = '" + v + "';" }

// upPipe runs MigrateUpDir over one pending file and returns the piped SQL.
func upPipe(t *testing.T, body string) string {
	t.Helper()
	dir := mkMigDir(t, map[string]string{"001_a.sql": body})
	sd := fakeDockerState(t, map[string]string{"q_legacy_exists": "yes", "q_ops_exists": "yes", "q_count": "1"})
	if n, err := MigrateUpDir(context.Background(), fakeCfg, dir); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	b, err := os.ReadFile(filepath.Join(sd, "pipe-0.sql"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// downPipe reverts 002_b.sql whose down file is downBody and returns the piped SQL.
func downPipe(t *testing.T, downBody string) string {
	t.Helper()
	dir := mkMigDir(t, map[string]string{
		"001_a.sql": "CREATE TABLE dir_a (id int);", "002_b.sql": "CREATE TABLE dir_b (id int);",
		"002_b_down.sql": downBody,
	})
	sd := fakeDockerState(t, downAnswers(t, dir, "002_b.sql"))
	if _, err := MigrateDownDir(context.Background(), fakeCfg, dir, 1); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(sd, "pipe-0.sql"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMigrateUpDir_StatementTimeoutDirective(t *testing.T) {
	s := upPipe(t, "-- nself:statement-timeout=600s\nCREATE TABLE dir_a (id int);")
	if !strings.Contains(s, stmtSet("600s")) || strings.Contains(s, "'60s'") || !strings.Contains(s, lock5) {
		t.Errorf("up SQL:\n%s", s)
	}
}

func TestMigrateUpDir_DefaultStatementTimeout(t *testing.T) {
	s := upPipe(t, "CREATE TABLE dir_a (id int);")
	if !strings.Contains(s, stmtSet("60s")) || !strings.Contains(s, lock5) {
		t.Errorf("up SQL:\n%s", s)
	}
}

func TestMigrateUpDir_StatementTimeoutZero(t *testing.T) {
	s := upPipe(t, "-- nself:statement-timeout=0\nCREATE TABLE dir_a (id int);")
	if !strings.Contains(s, stmtSet("0")) || strings.Contains(s, "'60s'") {
		t.Errorf("up SQL:\n%s", s)
	}
}

func TestMigrateDownDir_StatementTimeoutDirective(t *testing.T) {
	s := downPipe(t, "-- nself:statement-timeout=600s\nDROP TABLE dir_b;")
	if !strings.Contains(s, stmtSet("600s")) || strings.Contains(s, "'60s'") || !strings.Contains(s, lock5) {
		t.Errorf("down SQL:\n%s", s)
	}
}

func TestMigrateDownDir_DefaultStatementTimeout(t *testing.T) {
	s := downPipe(t, "DROP TABLE dir_b;")
	if !strings.Contains(s, stmtSet("60s")) || !strings.Contains(s, lock5) {
		t.Errorf("down SQL:\n%s", s)
	}
}

func TestMigrateDownDir_StatementTimeoutZero(t *testing.T) {
	s := downPipe(t, "-- nself:statement-timeout=0\nDROP TABLE dir_b;")
	if !strings.Contains(s, stmtSet("0")) || strings.Contains(s, "'60s'") {
		t.Errorf("down SQL:\n%s", s)
	}
}

func TestDirTxSQL_Timeouts(t *testing.T) {
	for _, timeout := range []string{"60s", "600s", "0", "5min"} {
		s := dirTxSQL("SELECT 1;", "-- ledger", timeout)
		if !strings.HasPrefix(s, "BEGIN;\n"+lock5+"\n"+stmtSet(timeout)+"\n") || !strings.HasSuffix(s, "COMMIT;\n") ||
			!strings.Contains(s, "-- ledger") || !strings.Contains(s, "SELECT 1;") {
			t.Errorf("timeout %q: unexpected SQL:\n%s", timeout, s)
		}
	}
}
