package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func ctxT() context.Context { return context.Background() }

func sqlFS(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for n, c := range files {
		m[n] = &fstest.MapFile{Data: []byte(c)}
	}
	return m
}

func TestApplyFresh(t *testing.T) {
	pool := newPool(t)
	schema := schemaFor(t, pool)
	files := map[string]string{
		"010_b.sql":        `INSERT INTO order_log(f) VALUES ('010_b');`,
		"002_a.sql":        `CREATE TABLE order_log(seq serial PRIMARY KEY, f text); INSERT INTO order_log(f) VALUES ('002_a');`,
		"1_c.sql":          `INSERT INTO order_log(f) VALUES ('1_c');`,
		"003_empty.sql":    "  \n",
		"002_a.down.sql":   `DROP TABLE order_log;`,
		"README.md":        `not sql`,
		"templates/x.sql":  `SELECT 1/0;`,
		"004_boom.sql.bak": `SELECT 1/0;`,
	}
	res, err := Apply(ctxT(), pool, Options{Schema: schema, FS: sqlFS(files)})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"002_a.sql", "003_empty.sql", "010_b.sql", "1_c.sql"}
	if !reflect.DeepEqual(res.Applied, want) {
		t.Fatalf("applied %v, want %v (lexical order, no .down.sql, no subdirs)", res.Applied, want)
	}
	// search_path is pinned: the unqualified CREATE landed in the plugin schema.
	if n := count(t, pool, `SELECT count(*) FROM information_schema.tables WHERE table_schema=$1 AND table_name='order_log'`, schema); n != 1 {
		t.Fatal("order_log not created inside the plugin schema")
	}
	rows, _ := pool.Query(ctxT(), `SELECT f FROM `+`"`+schema+`".order_log ORDER BY seq`)
	var got []string
	for rows.Next() {
		var f string
		_ = rows.Scan(&f)
		got = append(got, f)
	}
	rows.Close()
	if !reflect.DeepEqual(got, []string{"002_a", "010_b", "1_c"}) {
		t.Fatalf("execution order %v", got)
	}
	// Ledger: one row per file with the sha256 of the raw bytes.
	for name, content := range files {
		if _, ok := map[string]bool{"002_a.sql": true, "010_b.sql": true, "1_c.sql": true, "003_empty.sql": true}[name]; !ok {
			continue
		}
		sum := sha256.Sum256([]byte(content))
		var cs string
		if err := pool.QueryRow(ctxT(), `SELECT checksum FROM "`+schema+`".schema_migrations WHERE filename=$1`, name).Scan(&cs); err != nil {
			t.Fatalf("ledger row for %s: %v", name, err)
		}
		if cs != hex.EncodeToString(sum[:]) {
			t.Fatalf("checksum for %s = %s", name, cs)
		}
	}
	if n := count(t, pool, `SELECT count(*) FROM "`+schema+`".schema_migrations`); n != 4 {
		t.Fatalf("ledger has %d rows, want 4", n)
	}
}

func TestApplyRerun(t *testing.T) {
	pool := newPool(t)
	schema := schemaFor(t, pool)
	dir := t.TempDir()
	write := func(n, c string) {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("001.sql", `CREATE TABLE t(n int); INSERT INTO t VALUES (1);`)
	write("002.sql", `INSERT INTO t VALUES (2);`)
	opts := Options{Schema: schema, Dir: dir}
	if res, err := Apply(ctxT(), pool, opts); err != nil || len(res.Applied) != 2 {
		t.Fatalf("first boot: %v %v", res, err)
	}
	stamp := func() string {
		var s string
		_ = pool.QueryRow(ctxT(), `SELECT string_agg(filename||applied_at::text, ',' ORDER BY filename) FROM "`+schema+`".schema_migrations`).Scan(&s)
		return s
	}
	before := stamp()
	for i := 0; i < 3; i++ {
		res, err := Apply(ctxT(), pool, opts)
		if err != nil || len(res.Applied) != 0 {
			t.Fatalf("re-boot %d: applied %v err %v", i, res.Applied, err)
		}
	}
	if stamp() != before || count(t, pool, `SELECT count(*) FROM "`+schema+`".t`) != 2 {
		t.Fatal("re-boot changed the ledger or replayed a file")
	}
	// A new file is picked up alone; Dir inside an FS works too.
	write("003.sql", `INSERT INTO t VALUES (3);`)
	res, err := Apply(ctxT(), pool, Options{Schema: schema, FS: os.DirFS(filepath.Dir(dir)), Dir: filepath.Base(dir)})
	if err != nil || !reflect.DeepEqual(res.Applied, []string{"003.sql"}) {
		t.Fatalf("incremental: %v %v", res, err)
	}
}

func TestApplyFailureRollsBackAlone(t *testing.T) {
	pool := newPool(t)
	schema := schemaFor(t, pool)
	fsys := sqlFS(map[string]string{
		"001.sql": `CREATE TABLE t(n int);`,
		"002.sql": `CREATE TABLE half(n int); INSERT INTO t VALUES (1/0);`,
		"003.sql": `CREATE TABLE never(n int);`,
	})
	res, err := Apply(ctxT(), pool, Options{Schema: schema, FS: fsys})
	var fe *FileError
	if !errors.As(err, &fe) || fe.File != "002.sql" || !strings.Contains(err.Error(), "002.sql") {
		t.Fatalf("want FileError naming 002.sql, got %v", err)
	}
	if !reflect.DeepEqual(res.Applied, []string{"001.sql"}) {
		t.Fatalf("applied %v", res.Applied)
	}
	if n := count(t, pool, `SELECT count(*) FROM "`+schema+`".schema_migrations`); n != 1 {
		t.Fatalf("ledger rows %d, want only 001", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM information_schema.tables WHERE table_schema=$1 AND table_name IN ('half','never')`, schema); n != 0 {
		t.Fatal("failed file left a partial table, or a later file ran")
	}
	st, err := Status(ctxT(), pool, Options{Schema: schema, FS: fsys})
	if err != nil || st.Ready() || st.Applied != 1 || st.Expected != 3 {
		t.Fatalf("status %+v %v", st, err)
	}
}

func TestStatusHealthField(t *testing.T) {
	pool := newPool(t)
	schema := schemaFor(t, pool)
	opts := Options{Schema: schema, FS: sqlFS(map[string]string{"001.sql": `SELECT 1;`, "002.sql": `SELECT 2;`})}
	st, err := Status(ctxT(), pool, opts) // before any boot: no ledger yet
	if err != nil || !reflect.DeepEqual(st, Progress{Applied: 0, Expected: 2}) {
		t.Fatalf("before apply: %+v %v", st, err)
	}
	if _, err := Apply(ctxT(), pool, opts); err != nil {
		t.Fatal(err)
	}
	if st, err = Status(ctxT(), pool, opts); err != nil || !reflect.DeepEqual(st, Progress{Applied: 2, Expected: 2}) || !st.Ready() {
		t.Fatalf("after apply: %+v %v", st, err)
	}
	field := HealthField(st)
	if field != `"migrations":{"applied":2,"expected":2}` {
		t.Fatalf("health field %s", field)
	}
	var doc map[string]map[string]int
	if err := json.Unmarshal([]byte("{"+field+"}"), &doc); err != nil || doc["migrations"]["applied"] != 2 {
		t.Fatalf("field is not a valid JSON member: %v", err)
	}
	if b, _ := json.Marshal(st); string(b) != `{"applied":2,"expected":2}` {
		t.Fatalf("json tags: %s", b)
	}
}

func TestOptionsValidation(t *testing.T) {
	bad := []Options{
		{Schema: "public", Dir: "."},
		{Schema: "np_", Dir: "."},
		{Schema: `np_x"; DROP SCHEMA public; --`, Dir: "."},
		{Schema: "NP_x", Dir: "."},
		{Schema: "np_" + strings.Repeat("a", 70), Dir: "."},
		{Schema: "np_ok"},
		{Schema: "np_ok", Dir: ".", Between: func(context.Context) error { return nil }},
		{Schema: "np_ok", Dir: ".", SearchPath: []string{`public"; x`}},
		{Schema: "np_ok", Dir: ".", LockTimeout: -1},
	}
	for i, o := range bad {
		if _, err := newPlan(o); err == nil {
			t.Errorf("case %d (%+v) accepted", i, o)
		}
	}
	if p, err := newPlan(Options{Schema: "np_ok", Dir: ".", SearchPath: []string{"public"}}); err != nil || p.path != `"np_ok", "public"` {
		t.Errorf("good options: %v %v", p, err)
	}
}
