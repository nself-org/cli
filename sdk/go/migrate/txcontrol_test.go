package migrate

import (
	"errors"
	"strings"
	"testing"
)

func TestTxControlScanner(t *testing.T) {
	refused := map[string]string{
		"COMMIT":                          `CREATE TABLE a(n int); COMMIT;`,
		"commit lower":                    `create table a(n int); commit;`,
		"BEGIN":                           "BEGIN;\nCREATE TABLE a(n int);",
		"BEGIN TRANSACTION":               `begin transaction; select 1;`,
		"ROLLBACK":                        `SELECT 1; ROLLBACK;`,
		"END":                             `SELECT 1; END;`,
		"ABORT":                           `ABORT;`,
		"SAVEPOINT":                       `SAVEPOINT s1;`,
		"RELEASE":                         `RELEASE SAVEPOINT s1;`,
		"START TRANSACTION":               `START TRANSACTION ISOLATION LEVEL SERIALIZABLE;`,
		"PREPARE TRANSACTION":             `PREPARE TRANSACTION 'x';`,
		"after a comment":                 `/* hi */ COMMIT;`,
		"no trailing semicolon":           `SELECT 1; COMMIT`,
		"after a string with a semicolon": `INSERT INTO t VALUES ('a;b'); COMMIT;`,
		"after a function body":           "CREATE FUNCTION f() RETURNS int LANGUAGE sql AS $$ SELECT 1; $$;\nCOMMIT;",
		"after BEGIN ATOMIC body":         "CREATE FUNCTION f() RETURNS int LANGUAGE sql BEGIN ATOMIC SELECT 1; END;\nCOMMIT;",
	}
	for name, sql := range refused {
		if w, _ := findTxControl(sql); w == "" {
			t.Errorf("%s: %q was not refused", name, sql)
		}
	}
	if w, line := findTxControl("CREATE TABLE c1(n int);\n\nCOMMIT;\nCREATE TABLE c2(n int);"); w != "COMMIT" || line != 3 {
		t.Errorf("c1 case: got %q at line %d", w, line)
	}
	allowed := map[string]string{
		"word in a string":        `INSERT INTO t VALUES ('COMMIT; BEGIN; ROLLBACK');`,
		"escaped quote in string": `INSERT INTO t VALUES ('it''s; COMMIT');`,
		"E string backslash":      `INSERT INTO t VALUES (E'it\'s; COMMIT');`,
		"line comment":            "SELECT 1; -- COMMIT;\nSELECT 2;",
		"block comment":           `SELECT 1; /* COMMIT; */ SELECT 2;`,
		"nested block comment":    `SELECT 1; /* a /* COMMIT; */ COMMIT; */ SELECT 2;`,
		"dollar-quoted body":      `CREATE FUNCTION f() RETURNS void LANGUAGE plpgsql AS $$ BEGIN PERFORM 1; END; $$;`,
		"tagged dollar body":      `CREATE FUNCTION f() RETURNS void LANGUAGE plpgsql AS $body$ BEGIN COMMIT; END; $body$;`,
		"tag-like inner quote":    `DO $a$ BEGIN RAISE NOTICE $b$ COMMIT; $b$; END $a$;`,
		"quoted identifier":       `CREATE TABLE "commit"("end" int, "begin" int);`,
		"column named end":        `ALTER TABLE t ADD COLUMN end_at int;`,
		"identifier with prefix":  `CREATE TABLE commit_log(begin_at int); CREATE TABLE release_notes(n int);`,
		"BEGIN ATOMIC body": `CREATE FUNCTION f(i int) RETURNS int LANGUAGE sql
			BEGIN ATOMIC SELECT CASE WHEN i > 0 THEN 1 ELSE 0 END; SELECT 2; END;
			CREATE TABLE after_it(n int);`,
		"positional parameter": `PREPARE p AS SELECT $1; EXECUTE p(1);`,
		"empty":                ``,
		"only comments":        "-- nothing\n/* here */",
	}
	for name, sql := range allowed {
		if w, line := findTxControl(sql); w != "" {
			t.Errorf("%s: false positive %q at line %d in %q", name, w, line, sql)
		}
	}
}

func TestApplyRefusesTxControl(t *testing.T) {
	pool := newPool(t)
	schema := schemaFor(t, pool)
	// The reviewer's c1/COMMIT case: without the guard c1 stays committed
	// with no ledger row and every later boot fails on "already exists".
	fsys := sqlFS(map[string]string{
		"001.sql": `CREATE TABLE ok(n int);`,
		"002.sql": "CREATE TABLE c1(n int);\nCOMMIT;\nCREATE TABLE c2(n int);\nSELECT 1/0;",
	})
	res, err := Apply(ctxT(), pool, Options{Schema: schema, FS: fsys})
	var fe *FileError
	if !errors.As(err, &fe) || fe.File != "002.sql" || !errors.Is(err, ErrTxControl) || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("want FileError for 002.sql wrapping ErrTxControl, got %v", err)
	}
	if len(res.Applied) != 0 {
		t.Fatalf("nothing may run when a pending file is refused, applied %v", res.Applied)
	}
	if n := count(t, pool, `SELECT count(*) FROM information_schema.tables WHERE table_schema=$1 AND table_name IN ('ok','c1','c2')`, schema); n != 0 {
		t.Fatal("a refused set must change nothing")
	}
	if n := count(t, pool, `SELECT count(*) FROM "`+schema+`".schema_migrations`); n != 0 {
		t.Fatalf("ledger rows %d", n)
	}
	// Fixing the file lets the same database converge: no permanent breakage.
	fsys["002.sql"].Data = []byte("CREATE TABLE c1(n int);")
	if res, err = Apply(ctxT(), pool, Options{Schema: schema, FS: fsys}); err != nil || len(res.Applied) != 2 {
		t.Fatalf("after fixing the file: %v %v", res, err)
	}
}

// Defence in depth: a file that gets past the scanner but still ends its
// transaction is reported, not silently recorded.
func TestApplyFileDetectsEndedTransaction(t *testing.T) {
	pool := newPool(t)
	schema := schemaFor(t, pool)
	p, err := newPlan(Options{Schema: schema, FS: sqlFS(nil)})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := lockConn(ctxT(), pool, p.lockKey)
	if err != nil {
		t.Fatal(err)
	}
	defer closeConn(conn)
	if err := p.ensureLedger(ctxT(), conn); err != nil {
		t.Fatal(err)
	}
	err = p.applyFile(ctxT(), conn, file{name: "x.sql", sql: "CREATE TABLE x(n int); COMMIT;", sum: "s"})
	if !errors.Is(err, ErrTxControl) {
		t.Fatalf("want ErrTxControl, got %v", err)
	}
	if n := count(t, pool, `SELECT count(*) FROM "`+schema+`".schema_migrations`); n != 0 {
		t.Fatal("a file that ended its transaction must not be recorded")
	}
}
