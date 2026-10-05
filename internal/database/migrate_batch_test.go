package database

// migrate_batch_test.go — P7-LIVE-11: MigrateUpDir ensures the ledger tables
// once and reads the ledger once per run. Counting uses the docker.ExecStdin
// hook cross-checked against the fake docker's own call log; the integration
// test (INTEGRATION=1) compares ledger rows with the per-file path.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
)

// batchFiles returns n distinct flat migration files.
func batchFiles(n int) map[string]string {
	m := make(map[string]string, n)
	for i := 1; i <= n; i++ {
		m[fmt.Sprintf("%03d_t%d.sql", i, i)] = fmt.Sprintf("CREATE TABLE t%d (id int);", i)
	}
	return m
}

func sortedNames(files map[string]string) []string {
	var names []string
	for k := range files {
		names = append(names, k)
	}
	// zero-padded prefixes sort lexicographically
	for i := range names {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	return names
}

// appliedAnswer is the fake q_applied body listing names as applied.
func appliedAnswer(names []string) string {
	var l []string
	for _, n := range names {
		l = append(l, n+"|2026-10-04T10:00:00Z")
	}
	return strings.Join(l, "\n")
}

func countExecs(t *testing.T) (count func() int, restore func()) {
	t.Helper()
	n := 0
	restore = docker.SetExecHook(func(string, []string) { n++ })
	return func() int { return n }, restore
}

func TestMigrateUpDir_NoopRerunExecCountIndependentOfFileCount(t *testing.T) {
	var counts []int
	for _, n := range []int{1, 58} {
		files := batchFiles(n)
		dir := mkMigDir(t, files)
		names := sortedNames(files)
		sd := fakeDockerState(t, map[string]string{"q_applied": appliedAnswer(names), "q_checksums": dirSums(t, dir, names...)})
		execs, restore := countExecs(t)
		got, err := MigrateUpDir(context.Background(), fakeCfg, dir)
		restore()
		if err != nil || got != 0 {
			t.Fatalf("n=%d: rerun = %d, %v", n, got, err)
		}
		real := countPrefix(recordedCalls(t, sd), "CALL ")
		if execs() != real || execs() == 0 {
			t.Fatalf("n=%d: hook counted %d execs but docker saw %d (seam must see every exec)", n, execs(), real)
		}
		if execs() > 5 {
			t.Errorf("n=%d: %d execs on a no-op rerun, want <= 5", n, execs())
		}
		if c := countPrefix(recordedCalls(t, sd), "CALL QUERY SELECT name || '|' || applied_at"); c != 1 {
			t.Errorf("n=%d: ledger read %d times, want 1", n, c)
		}
		counts = append(counts, execs())
	}
	if counts[0] != counts[1] {
		t.Errorf("exec count depends on file count: 1 file = %d, 58 files = %d", counts[0], counts[1])
	}
}

func TestMigrateUpDirProgress_OneLedgerReadAndProgressPerPendingFile(t *testing.T) {
	files := batchFiles(4)
	dir := mkMigDir(t, files)
	names := sortedNames(files)
	sd := fakeDockerState(t, map[string]string{"q_applied": appliedAnswer(names[:1]), "q_checksums": dirSums(t, dir, names[0]), "q_count": "1"})
	var got []string
	n, err := MigrateUpDirProgress(context.Background(), fakeCfg, dir, func(i, total int, f string) {
		got = append(got, fmt.Sprintf("[%d/%d] %s", i, total, f))
	})
	if err != nil || n != 3 {
		t.Fatalf("applied %d, %v; want 3", n, err)
	}
	want := []string{"[1/3] 002_t2.sql", "[2/3] 003_t3.sql", "[3/3] 004_t4.sql"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("progress = %v, want %v", got, want)
	}
	calls := recordedCalls(t, sd)
	if c := countPrefix(calls, "CALL QUERY SELECT name || '|' || applied_at"); c != 1 {
		t.Errorf("applied-names read %d times, want 1", c)
	}
	if c := countPrefix(calls, "CALL QUERY SELECT name || '|' || checksum"); c != 1 {
		t.Errorf("checksums read %d times, want 1", c)
	}
	if c := countPrefix(calls, "CALL PIPE"); c != 3 {
		t.Errorf("%d transactions, want one per pending file (3)", c)
	}
	if c := countPrefix(calls, "CALL EXEC"); c != 1 {
		t.Errorf("%d table ensures, want 1", c)
	}
	for i := 0; i < 3; i++ {
		b, _ := os.ReadFile(filepath.Join(sd, fmt.Sprintf("pipe-%d.sql", i)))
		if strings.Count(string(b), "BEGIN;") != 1 || !strings.Contains(string(b), names[i+1]) {
			t.Errorf("pipe-%d is not file %s in its own transaction:\n%s", i, names[i+1], b)
		}
	}
}

func TestMigrateUpDir_ChecksumMismatchStillFails(t *testing.T) {
	files := batchFiles(2)
	dir := mkMigDir(t, files)
	names := sortedNames(files)
	sd := fakeDockerState(t, map[string]string{"q_applied": appliedAnswer(names[:1]), "q_checksums": names[0] + "|deadbeef"})
	_, err := MigrateUpDir(context.Background(), fakeCfg, dir)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("want checksum mismatch, got %v", err)
	}
	if countPrefix(recordedCalls(t, sd), "CALL PIPE") != 0 {
		t.Error("applied a file after a checksum mismatch on an earlier one")
	}
}

// failingDocker makes every query whose text contains match fail with exit 1.
func failingDocker(t *testing.T, match string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a /bin/sh script; Windows cannot execute it")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nfor last; do :; done\ncase \"$last\" in *'" + match + "'*) echo kaboom >&2; exit 1;; esac\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(pathEnvVar, bin+string(os.PathListSeparator)+os.Getenv(pathEnvVar))
}

func TestMigrateUpDir_LedgerReadErrorsAreReturned(t *testing.T) {
	dir := mkMigDir(t, batchFiles(1))
	for _, c := range []struct{ match, want string }{
		{"applied_at FROM", "check applied migrations"},
		{"|| checksum FROM", "read migration checksums"},
		{"CREATE SCHEMA IF NOT EXISTS np_common", "ensure ledger tables"},
	} {
		failingDocker(t, c.match)
		_, err := MigrateUpDir(context.Background(), fakeCfg, dir)
		if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "kaboom") {
			t.Errorf("failing %q: want %q with stderr, got %v", c.match, c.want, err)
		}
	}
}

func TestLedgerSnapshot_DuplicateNameChecksumsJoinLikeTheOldQuery(t *testing.T) {
	sd := fakeDockerState(t, map[string]string{"q_checksums": "a.sql|c1\na.sql|c2\nb.sql|c3"})
	_ = sd
	led, err := readLedger(context.Background(), fakeCfg)
	if err != nil {
		t.Fatal(err)
	}
	if led.sums["a.sql"] != "c1\nc2" || led.sums["b.sql"] != "c3" {
		t.Errorf("sums = %v", led.sums)
	}
}

// perFileMigrateUpDir is the pre-P7-LIVE-11 MigrateUpDir loop: the per-file
// path (applyFile with no shared ledger) is unchanged code, so this is the
// reference the batched run is compared with.
func perFileMigrateUpDir(ctx context.Context, cfg *config.Config, dir string) (int, error) {
	files, err := scanMigrations(dir)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, f := range files {
		skipped, err := applyFile(ctx, cfg, f, true, nil)
		if err != nil {
			return count, err
		}
		if !skipped {
			count++
		}
	}
	return count, nil
}

func ledgerDump(t *testing.T, cfg *config.Config) string {
	t.Helper()
	var parts []string
	for _, q := range []string{
		"SELECT string_agg(name, ',' ORDER BY name) FROM np_common.schema_versions",
		"SELECT string_agg(id || ':' || name || ':' || checksum, ',' ORDER BY id) FROM nself_ops.migrations",
		"SELECT string_agg(table_schema || '.' || table_name || '.' || column_name || ':' || data_type || ':' || is_nullable || ':' || coalesce(column_default, ''), ',' ORDER BY table_schema, table_name, ordinal_position) FROM information_schema.columns WHERE table_schema IN ('np_common', 'nself_ops')",
		"SELECT string_agg(conrelid::regclass::text || ':' || contype::text || ':' || pg_get_constraintdef(oid), ',' ORDER BY conrelid::regclass::text, contype) FROM pg_constraint WHERE connamespace IN ('np_common'::regnamespace, 'nself_ops'::regnamespace)",
		"SELECT count(*) FROM pg_tables WHERE tablename ~ '^t[0-9]+$'",
	} {
		parts = append(parts, pgScalar(t, cfg, q))
	}
	return strings.Join(parts, "\n")
}

func TestMigrateUpDirIntegration_BatchedEqualsPerFile(t *testing.T) {
	cfgNew, cfgOld := startDirPG(t), startDirPG(t)
	ctx := context.Background()
	files := batchFiles(58)
	files["030_t30.sql"] = "BEGIN;\nCREATE TABLE t30 (id int);\nCOMMIT;\n"
	files["031_t31.sql"] = "CREATE TABLE t31 (id int);\nCREATE INDEX CONCURRENTLY t31_i ON t31 (id);\n"
	dir := mkMigDir(t, files)

	run := func(label string, cfg *config.Config, fn func() (int, error)) (n int, execs int, took time.Duration) {
		c, restore := countExecs(t)
		defer restore()
		start := time.Now()
		n, err := fn()
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		return n, c(), time.Since(start)
	}
	var progress []string
	nNew, eNew, dNew := run("batched apply", cfgNew, func() (int, error) {
		return MigrateUpDirProgress(ctx, cfgNew, dir, func(i, tot int, f string) { progress = append(progress, fmt.Sprintf("[%d/%d] %s", i, tot, f)) })
	})
	nOld, eOld, dOld := run("per-file apply", cfgOld, func() (int, error) { return perFileMigrateUpDir(ctx, cfgOld, dir) })
	if nNew != 58 || nOld != 58 {
		t.Fatalf("applied new=%d old=%d, want 58 both", nNew, nOld)
	}
	if len(progress) != 58 || progress[0] != "[1/58] 001_t1.sql" || progress[57] != "[58/58] 058_t58.sql" {
		t.Errorf("progress lines wrong: %d, first %q last %q", len(progress), progress[0], progress[len(progress)-1])
	}
	if a, b := ledgerDump(t, cfgNew), ledgerDump(t, cfgOld); a != b || !strings.Contains(a, "t31") {
		t.Fatalf("ledger/tables differ after apply:\nbatched:\n%s\nper-file:\n%s", a, b)
	}

	// No-op rerun: identical result, and the batched exec count is small.
	rNew, rEnew, rDnew := run("batched rerun", cfgNew, func() (int, error) { return MigrateUpDir(ctx, cfgNew, dir) })
	rOld, rEold, rDold := run("per-file rerun", cfgOld, func() (int, error) { return perFileMigrateUpDir(ctx, cfgOld, dir) })
	if rNew != 0 || rOld != 0 {
		t.Fatalf("rerun applied new=%d old=%d, want 0", rNew, rOld)
	}
	if rEnew > 5 || rEold <= 58 {
		t.Errorf("rerun execs: batched %d (want <= 5), per-file %d (want > 58)", rEnew, rEold)
	}
	t.Logf("58 files apply: batched %d execs %v, per-file %d execs %v", eNew, dNew, eOld, dOld)
	t.Logf("58 files no-op rerun: batched %d execs %v, per-file %d execs %v", rEnew, rDnew, rEold, rDold)
	if a, b := ledgerDump(t, cfgNew), ledgerDump(t, cfgOld); a != b {
		t.Fatalf("ledger differs after rerun")
	}

	// A file edited after apply fails the same way in both.
	if err := os.WriteFile(filepath.Join(dir, "010_t10.sql"), []byte("CREATE TABLE t10 (id int, extra int);"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, errNew := MigrateUpDir(ctx, cfgNew, dir)
	_, errOld := perFileMigrateUpDir(ctx, cfgOld, dir)
	if errNew == nil || errOld == nil || errNew.Error() != errOld.Error() || !strings.Contains(errNew.Error(), "checksum mismatch") {
		t.Fatalf("checksum mismatch differs: batched %v / per-file %v", errNew, errOld)
	}
}
