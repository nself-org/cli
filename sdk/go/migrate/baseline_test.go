package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func sumOf(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := pool.Exec(ctxT(), sql); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func TestApplyRefusesObjectsWithoutLedger(t *testing.T) {
	pool := newPool(t)
	schema := schemaFor(t, pool)
	files := map[string]string{
		"001.sql": `CREATE TABLE hand(n int);`, // would fail with 42P07 if replayed
		"002.sql": `INSERT INTO hand VALUES (2);`,
	}
	mustExec(t, pool, `CREATE SCHEMA "`+schema+`"`)
	if _, err := Apply(ctxT(), pool, Options{Schema: schema, FS: sqlFS(files)}); err != nil {
		t.Fatalf("an empty pre-created schema (what plugin install leaves) must work: %v", err)
	}
	if _, err := pool.Exec(ctxT(), `DROP SCHEMA "`+schema+`" CASCADE`); err != nil {
		t.Fatal(err)
	}
	// A hand-built schema: objects, no ledger.
	mustExec(t, pool, `CREATE SCHEMA "`+schema+`"; CREATE TABLE "`+schema+`".hand(n int); INSERT INTO "`+schema+`".hand VALUES (1)`)
	res, err := Apply(ctxT(), pool, Options{Schema: schema, FS: sqlFS(files)})
	if !errors.Is(err, ErrUnledgered) || !strings.Contains(err.Error(), "Baseline") || len(res.Applied) != 0 {
		t.Fatalf("want ErrUnledgered pointing to Baseline, got %v %v", res, err)
	}
	if n := count(t, pool, `SELECT (to_regclass('"`+schema+`".schema_migrations') IS NOT NULL)::int`); n != 0 {
		t.Fatal("refusal must not create the ledger")
	}
	if n := count(t, pool, `SELECT count(*) FROM "`+schema+`".hand`); n != 1 {
		t.Fatal("refusal must not run any file")
	}
	// Baseline records both files, unexecuted, with checksums.
	res, err = Baseline(ctxT(), pool, Options{Schema: schema, FS: sqlFS(files)})
	if err != nil || !reflect.DeepEqual(res.Baselined, []string{"001.sql", "002.sql"}) {
		t.Fatalf("baseline: %v %v", res, err)
	}
	if n := count(t, pool, `SELECT count(*) FROM "`+schema+`".hand`); n != 1 {
		t.Fatal("Baseline executed a file")
	}
	var cs string
	_ = pool.QueryRow(ctxT(), `SELECT checksum FROM "`+schema+`".schema_migrations WHERE filename='002.sql'`).Scan(&cs)
	if cs != sumOf(files["002.sql"]) {
		t.Fatalf("checksum %s", cs)
	}
	if res, err = Apply(ctxT(), pool, Options{Schema: schema, FS: sqlFS(files)}); err != nil || len(res.Applied) != 0 {
		t.Fatalf("apply after baseline: %v %v", res, err)
	}
	if res, err = Baseline(ctxT(), pool, Options{Schema: schema, FS: sqlFS(files)}); err != nil || len(res.Baselined) != 0 {
		t.Fatalf("baseline is idempotent: %v %v", res, err)
	}
	// A file added later runs normally.
	files["003.sql"] = `INSERT INTO hand VALUES (3);`
	if res, err = Apply(ctxT(), pool, Options{Schema: schema, FS: sqlFS(files)}); err != nil || !reflect.DeepEqual(res.Applied, []string{"003.sql"}) {
		t.Fatalf("new file after baseline: %v %v", res, err)
	}
}

// claw's real ledger: np_claw.schema_migrations (filename TEXT PRIMARY KEY,
// applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()), written by hand on prod.
func TestBaselineLegacyClawLedger(t *testing.T) {
	pool := newPool(t)
	schema := schemaFor(t, pool)
	files := map[string]string{
		"001_a.sql": `CREATE TABLE sessions(n int);`,
		"002_b.sql": `ALTER TABLE sessions ADD COLUMN x int;`,
		"003_c.sql": `INSERT INTO sessions(n) VALUES (3);`,
	}
	mustExec(t, pool, `CREATE SCHEMA "`+schema+`";
		CREATE TABLE "`+schema+`".sessions(n int, x int);
		CREATE TABLE "`+schema+`".schema_migrations (
			filename    TEXT PRIMARY KEY,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW());
		INSERT INTO "`+schema+`".schema_migrations(filename) VALUES ('001_a.sql'), ('002_b.sql'), ('000_gone.sql')`)
	opts := Options{Schema: schema, FS: sqlFS(files)}

	// Apply refuses, pointing at Baseline; Status still reads the legacy ledger by name.
	if _, err := Apply(ctxT(), pool, opts); !errors.Is(err, ErrLegacyLedger) || !strings.Contains(err.Error(), "Baseline") {
		t.Fatalf("want ErrLegacyLedger pointing to Baseline, got %v", err)
	}
	st, err := Status(ctxT(), pool, opts)
	if err != nil || st.Applied != 2 || st.Expected != 3 || st.Ready() {
		t.Fatalf("status on legacy ledger: %+v %v", st, err)
	}

	res, err := Baseline(ctxT(), pool, opts)
	if err != nil || !reflect.DeepEqual(res.Baselined, []string{"003_c.sql"}) {
		t.Fatalf("baseline: %v %v", res, err)
	}
	// Existing rows got checksums from the current files; the row for a file
	// that is gone got the empty checksum; applied_at was kept.
	for name, want := range map[string]string{"001_a.sql": sumOf(files["001_a.sql"]),
		"002_b.sql": sumOf(files["002_b.sql"]), "003_c.sql": sumOf(files["003_c.sql"]), "000_gone.sql": ""} {
		var cs string
		if err := pool.QueryRow(ctxT(), `SELECT checksum FROM "`+schema+`".schema_migrations WHERE filename=$1`, name).Scan(&cs); err != nil || cs != want {
			t.Fatalf("%s: checksum %q err %v, want %q", name, cs, err, want)
		}
	}
	if n := count(t, pool, `SELECT count(*) FROM pg_attribute WHERE attrelid = to_regclass('"`+schema+`".schema_migrations') AND attname='checksum' AND attnotnull`); n != 1 {
		t.Fatal("checksum column must be NOT NULL after the upgrade")
	}
	if n := count(t, pool, `SELECT count(*) FROM "`+schema+`".sessions`); n != 0 {
		t.Fatal("Baseline executed a file")
	}
	if res, err := Apply(ctxT(), pool, opts); err != nil || len(res.Applied) != 0 {
		t.Fatalf("apply after upgrade: %v %v", res, err)
	}
	// The deleted file shows in Status, so the plugin is not "ready" until a
	// human decides what to do about it.
	st, err = Status(ctxT(), pool, opts)
	if err != nil || st.Applied != 3 || !reflect.DeepEqual(st.Deleted, []string{"000_gone.sql"}) || st.Ready() {
		t.Fatalf("status after baseline: %+v %v", st, err)
	}
	if got := HealthField(st); got != `"migrations":{"applied":3,"expected":3,"deleted":1}` {
		t.Fatalf("health field %s", got)
	}
}

func TestApplyRefusesOutOfOrderFile(t *testing.T) {
	pool := newPool(t)
	schema := schemaFor(t, pool)
	files := map[string]string{"0010_b.sql": `CREATE TABLE b(n int);`}
	if _, err := Apply(ctxT(), pool, Options{Schema: schema, FS: sqlFS(files)}); err != nil {
		t.Fatal(err)
	}
	files["0005_late.sql"] = `CREATE TABLE late(n int);`
	res, err := Apply(ctxT(), pool, Options{Schema: schema, FS: sqlFS(files)})
	var oe *OutOfOrderError
	if !errors.As(err, &oe) || oe.File != "0005_late.sql" || oe.After != "0010_b.sql" || len(res.Applied) != 0 {
		t.Fatalf("want OutOfOrderError, got %v %v", res, err)
	}
	if n := count(t, pool, `SELECT count(*) FROM information_schema.tables WHERE table_schema=$1 AND table_name='late'`, schema); n != 0 {
		t.Fatal("out-of-order file ran")
	}
	files["0011_after.sql"] = `CREATE TABLE after(n int);`
	delete(files, "0005_late.sql")
	if res, err = Apply(ctxT(), pool, Options{Schema: schema, FS: sqlFS(files)}); err != nil || len(res.Applied) != 1 {
		t.Fatalf("in-order file: %v %v", res, err)
	}
}

func TestStatusReportsDriftAndDeleted(t *testing.T) {
	pool := newPool(t)
	schema := schemaFor(t, pool)
	files := map[string]string{"001.sql": `CREATE TABLE a(n int);`, "002.sql": `CREATE TABLE b(n int);`, "003.sql": `CREATE TABLE c(n int);`}
	if _, err := Apply(ctxT(), pool, Options{Schema: schema, FS: sqlFS(files)}); err != nil {
		t.Fatal(err)
	}
	st, err := Status(ctxT(), pool, Options{Schema: schema, FS: sqlFS(files)})
	if err != nil || !st.Ready() || HealthField(st) != `"migrations":{"applied":3,"expected":3}` {
		t.Fatalf("clean: %+v %v", st, err)
	}
	files["001.sql"] = `CREATE TABLE a(n bigint);` // drift
	delete(files, "003.sql")                       // deleted
	st, err = Status(ctxT(), pool, Options{Schema: schema, FS: sqlFS(files)})
	want := Progress{Applied: 1, Expected: 2, Drifted: 1, Deleted: []string{"003.sql"}}
	if err != nil || !reflect.DeepEqual(st, want) || st.Ready() {
		t.Fatalf("got %+v %v, want %+v", st, err, want)
	}
	if got := HealthField(st); got != `"migrations":{"applied":1,"expected":2,"drifted":1,"deleted":1}` {
		t.Fatalf("health field %s", got)
	}
}
