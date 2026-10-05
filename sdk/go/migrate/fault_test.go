package migrate

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestApplyConcurrent(t *testing.T) {
	pool := newPool(t)
	schema := schemaFor(t, pool)
	fsys := sqlFS(map[string]string{
		"001.sql": `CREATE TABLE hits(f text); SELECT pg_sleep(1); INSERT INTO hits VALUES ('001');`,
		"002.sql": `INSERT INTO hits VALUES ('002');`,
		"003.sql": `INSERT INTO hits VALUES ('003');`,
	})
	const replicas = 4
	var wg sync.WaitGroup
	results := make([]Result, replicas)
	errs := make([]error, replicas)
	for i := 0; i < replicas; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = Apply(ctxT(), pool, Options{Schema: schema, FS: fsys})
		}(i)
	}
	wg.Wait()
	var all []string
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("replica %d: %v", i, errs[i])
		}
		all = append(all, results[i].Applied...)
	}
	sort.Strings(all)
	if !reflect.DeepEqual(all, []string{"001.sql", "002.sql", "003.sql"}) {
		t.Fatalf("files applied across replicas: %v (each exactly once)", all)
	}
	if n := count(t, pool, `SELECT count(*) FROM "`+schema+`".hits`); n != 3 {
		t.Fatalf("hits rows %d, want 3", n)
	}

	// The lock is per plugin: while plugin A holds its lock, plugin B boots.
	a, b := schema+"_a", schema+"_b"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctxT(), `DROP SCHEMA IF EXISTS "`+a+`" CASCADE`)
		_, _ = pool.Exec(ctxT(), `DROP SCHEMA IF EXISTS "`+b+`" CASCADE`)
	})
	one := sqlFS(map[string]string{"001.sql": `SELECT 1;`})
	holding, release := make(chan struct{}), make(chan struct{})
	aDone := make(chan error, 1)
	go func() {
		_, err := Apply(ctxT(), pool, Options{Schema: a, FS: one, Tolerant: true,
			Between: func(context.Context) error { close(holding); <-release; return nil }})
		aDone <- err
	}()
	<-holding
	ctx, cancel := context.WithTimeout(ctxT(), 20*time.Second)
	defer cancel()
	if _, err := Apply(ctx, pool, Options{Schema: b, FS: one}); err != nil {
		t.Fatalf("plugin B blocked behind plugin A's lock: %v", err)
	}
	// A second boot of A waits for A, and gives up when its ctx ends.
	wctx, wcancel := context.WithTimeout(ctxT(), 700*time.Millisecond)
	defer wcancel()
	if _, err := Apply(wctx, pool, Options{Schema: a, FS: one}); err == nil {
		t.Fatal("second boot of A did not wait for the advisory lock")
	}
	close(release)
	if err := <-aDone; err != nil {
		t.Fatal(err)
	}
}

func TestApplyKill(t *testing.T) {
	slow := sqlFS(map[string]string{
		"001.sql": `CREATE TABLE t(n int);`,
		"002.sql": `CREATE TABLE slow(n int); INSERT INTO t VALUES (1); SELECT pg_sleep(3); INSERT INTO t VALUES (2);`,
	})
	for _, mode := range []string{"context-cancel", "terminate-backend"} {
		t.Run(mode, func(t *testing.T) {
			pool := newPool(t)
			schema := schemaFor(t, pool)
			opts := Options{Schema: schema, FS: slow}
			errc := make(chan error, 1)
			ctx, cancel := context.WithCancel(ctxT())
			defer cancel()
			go func() { _, err := Apply(ctx, pool, opts); errc <- err }()
			deadline := time.Now().Add(20 * time.Second)
			for count(t, pool, `SELECT count(*) FROM pg_stat_activity WHERE query LIKE '%pg_sleep(3)%' AND pid <> pg_backend_pid() AND state='active'`) == 0 {
				if time.Now().After(deadline) {
					t.Fatal("slow migration never started")
				}
				time.Sleep(50 * time.Millisecond)
			}
			if mode == "context-cancel" {
				cancel()
			} else {
				count(t, pool, `SELECT count(pg_terminate_backend(pid)) FROM pg_stat_activity WHERE query LIKE '%pg_sleep(3)%' AND pid <> pg_backend_pid()`)
			}
			if err := <-errc; err == nil {
				t.Fatal("interrupted Apply reported success")
			}
			// No half-applied file: 001 is recorded, 002 left nothing behind.
			if n := count(t, pool, `SELECT count(*) FROM "`+schema+`".schema_migrations`); n != 1 {
				t.Fatalf("ledger rows %d after kill, want 1", n)
			}
			if n := count(t, pool, `SELECT count(*) FROM information_schema.tables WHERE table_schema=$1 AND table_name='slow'`, schema); n != 0 {
				t.Fatal("killed file left its table behind")
			}
			// Re-run converges, and no advisory lock is stranded.
			res, err := Apply(ctxT(), pool, opts)
			if err != nil || !reflect.DeepEqual(res.Applied, []string{"002.sql"}) {
				t.Fatalf("re-run: %v %v", res, err)
			}
			if n := count(t, pool, `SELECT count(*) FROM "`+schema+`".t`); n != 2 {
				t.Fatalf("t has %d rows, want 2", n)
			}
			if n := count(t, pool, `SELECT count(*) FROM pg_locks WHERE locktype='advisory'`); n != 0 {
				t.Fatalf("%d advisory locks left held", n)
			}
		})
	}
}

func TestApplyChecksum(t *testing.T) {
	pool := newPool(t)
	schema := schemaFor(t, pool)
	if _, err := Apply(ctxT(), pool, Options{Schema: schema, FS: sqlFS(map[string]string{
		"001.sql": `CREATE TABLE t(n int);`, "002.sql": `INSERT INTO t VALUES (1);`})}); err != nil {
		t.Fatal(err)
	}
	edited := sqlFS(map[string]string{
		"001.sql": `CREATE TABLE t(n int, extra text);`, "002.sql": `INSERT INTO t VALUES (1);`,
		"003.sql": `INSERT INTO t VALUES (3);`})
	res, err := Apply(ctxT(), pool, Options{Schema: schema, FS: edited})
	var ce *ChecksumError
	if !errors.As(err, &ce) || ce.File != "001.sql" || !strings.Contains(err.Error(), "001.sql") {
		t.Fatalf("want ChecksumError for 001.sql, got %v", err)
	}
	if len(res.Applied) != 0 || count(t, pool, `SELECT count(*) FROM "`+schema+`".t`) != 1 {
		t.Fatal("drift must stop the boot before any new file runs")
	}
}

func TestApplyTolerant(t *testing.T) {
	pool := newPool(t)
	files := map[string]string{
		"001.sql": `CREATE TABLE a(n int);`,
		// 002 needs go_table, which only the Go callback creates; 003 needs 002's column.
		"002.sql": `ALTER TABLE go_table ADD COLUMN x int;`,
		"003.sql": `INSERT INTO go_table(x) VALUES (7);`,
		"004.sql": `CREATE TABLE b(n int);`,
	}
	t.Run("strict-fails-naming-file", func(t *testing.T) {
		schema := schemaFor(t, pool)
		res, err := Apply(ctxT(), pool, Options{Schema: schema, FS: sqlFS(files)})
		var fe *FileError
		var pe *pgconn.PgError
		if !errors.As(err, &fe) || fe.File != "002.sql" || !errors.As(err, &pe) || pe.Code != "42P01" {
			t.Fatalf("strict mode: %v", err)
		}
		if !reflect.DeepEqual(res.Applied, []string{"001.sql"}) {
			t.Fatalf("applied %v", res.Applied)
		}
	})
	t.Run("tolerant-defers-then-applies-after-callback", func(t *testing.T) {
		schema := schemaFor(t, pool)
		ran := false
		res, err := Apply(ctxT(), pool, Options{Schema: schema, FS: sqlFS(files), Tolerant: true,
			Between: func(ctx context.Context) error {
				ran = true
				if n := count(t, pool, `SELECT count(*) FROM "`+schema+`".schema_migrations`); n != 2 {
					t.Errorf("before the callback only 001 and 004 are applied, ledger has %d", n)
				}
				_, err := pool.Exec(ctx, `CREATE TABLE "`+schema+`".go_table(id serial)`)
				return err
			}})
		if err != nil || !ran {
			t.Fatalf("tolerant: ran=%v err=%v", ran, err)
		}
		if !reflect.DeepEqual(res.Applied, []string{"001.sql", "004.sql", "002.sql", "003.sql"}) {
			t.Fatalf("applied %v", res.Applied)
		}
		if n := count(t, pool, `SELECT x FROM "`+schema+`".go_table`); n != 7 {
			t.Fatal("003 did not run after 002")
		}
	})
	t.Run("still-missing-after-callback-is-fatal", func(t *testing.T) {
		schema := schemaFor(t, pool)
		_, err := Apply(ctxT(), pool, Options{Schema: schema, FS: sqlFS(files), Tolerant: true,
			Between: func(context.Context) error { return nil }})
		var fe *FileError
		if !errors.As(err, &fe) || fe.File != "002.sql" {
			t.Fatalf("want fatal FileError for 002.sql, got %v", err)
		}
		if n := count(t, pool, `SELECT count(*) FROM "`+schema+`".schema_migrations WHERE filename IN ('002.sql','003.sql')`); n != 0 {
			t.Fatal("deferred files must stay unrecorded")
		}
	})
	t.Run("between-error-fails-boot", func(t *testing.T) {
		schema := schemaFor(t, pool)
		_, err := Apply(ctxT(), pool, Options{Schema: schema, FS: sqlFS(files), Tolerant: true,
			Between: func(context.Context) error { return errors.New("go migration exploded") }})
		if err == nil || !strings.Contains(err.Error(), "go migration exploded") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("non-dependency-error-is-not-deferred", func(t *testing.T) {
		schema := schemaFor(t, pool)
		_, err := Apply(ctxT(), pool, Options{Schema: schema, Tolerant: true,
			FS: sqlFS(map[string]string{"001.sql": `SELECT 1/0;`})})
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != "22012" {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("deferrable-codes", func(t *testing.T) {
		for _, c := range []string{"42P01", "42703", "42704"} {
			if !isDeferrable(&pgconn.PgError{Code: c}) {
				t.Errorf("%s should defer", c)
			}
		}
		for _, c := range []string{"42P07", "23505", "57014", "22012"} {
			if isDeferrable(&pgconn.PgError{Code: c}) {
				t.Errorf("%s must not defer", c)
			}
		}
		if isDeferrable(errors.New("plain")) {
			t.Error("plain error deferred")
		}
	})
}

func TestApplyTimeouts(t *testing.T) {
	pool := newPool(t)
	probe := map[string]string{"001.sql": `CREATE TABLE probe AS SELECT
		current_setting('lock_timeout') AS lt, current_setting('statement_timeout') AS st,
		current_setting('search_path') AS sp;`}
	read := func(schema string) (lt, st, sp string) {
		if err := pool.QueryRow(ctxT(), `SELECT lt, st, sp FROM "`+schema+`".probe`).Scan(&lt, &st, &sp); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Run("defaults", func(t *testing.T) {
		schema := schemaFor(t, pool)
		if _, err := Apply(ctxT(), pool, Options{Schema: schema, FS: sqlFS(probe)}); err != nil {
			t.Fatal(err)
		}
		lt, st, sp := read(schema)
		if lt != "5s" || st != "1min" || sp != `"`+schema+`"` {
			t.Fatalf("lock_timeout=%s statement_timeout=%s search_path=%s", lt, st, sp)
		}
	})
	t.Run("overrides-and-search-path", func(t *testing.T) {
		schema := schemaFor(t, pool)
		if _, err := Apply(ctxT(), pool, Options{Schema: schema, FS: sqlFS(probe), LockTimeout: 3 * time.Second,
			StatementTimeout: 45 * time.Second, SearchPath: []string{"public"}}); err != nil {
			t.Fatal(err)
		}
		lt, st, sp := read(schema)
		if lt != "3s" || st != "45s" || sp != `"`+schema+`", "public"` {
			t.Fatalf("lock_timeout=%s statement_timeout=%s search_path=%s", lt, st, sp)
		}
	})
	t.Run("session-settings-do-not-leak-to-pool", func(t *testing.T) {
		schema := schemaFor(t, pool)
		if _, err := Apply(ctxT(), pool, Options{Schema: schema, FS: sqlFS(map[string]string{
			"001.sql": `SET search_path = public;`})}); err != nil {
			t.Fatal(err)
		}
		var sp string
		_ = pool.QueryRow(ctxT(), `SHOW search_path`).Scan(&sp)
		if strings.Contains(sp, schema) {
			t.Fatalf("pool session polluted: %s", sp)
		}
	})
	t.Run("statement-timeout-enforced", func(t *testing.T) {
		schema := schemaFor(t, pool)
		_, err := Apply(ctxT(), pool, Options{Schema: schema, StatementTimeout: 300 * time.Millisecond,
			FS: sqlFS(map[string]string{"001.sql": `SELECT pg_sleep(5);`})})
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != "57014" {
			t.Fatalf("got %v", err)
		}
		if n := count(t, pool, `SELECT count(*) FROM "`+schema+`".schema_migrations`); n != 0 {
			t.Fatal("timed-out file was recorded")
		}
	})
	t.Run("lock-timeout-enforced", func(t *testing.T) {
		schema := schemaFor(t, pool)
		busy := map[string]string{"001.sql": `CREATE TABLE busy(n int);`}
		if _, err := Apply(ctxT(), pool, Options{Schema: schema, FS: sqlFS(busy)}); err != nil {
			t.Fatal(err)
		}
		tx, err := pool.Begin(ctxT())
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctxT())
		if _, err := tx.Exec(ctxT(), `LOCK TABLE "`+schema+`".busy IN ACCESS EXCLUSIVE MODE`); err != nil {
			t.Fatal(err)
		}
		busy["002.sql"] = `ALTER TABLE busy ADD COLUMN x int;`
		start := time.Now()
		_, err = Apply(ctxT(), pool, Options{Schema: schema, LockTimeout: 400 * time.Millisecond, FS: sqlFS(busy)})
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != "55P03" || time.Since(start) > 10*time.Second {
			t.Fatalf("got %v after %v", err, time.Since(start))
		}
	})
}

// A role-level statement_timeout / lock_timeout must not cut the wait behind
// another replica short: the wait is bounded by ctx alone.
func TestApplyLockWaitIgnoresDefaultTimeouts(t *testing.T) {
	cfg, err := pgxpool.ParseConfig(testDSN)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "200"
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = "200"
	pool, err := pgxpool.NewWithConfig(ctxT(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	schema := schemaFor(t, pool)
	one := sqlFS(map[string]string{"001.sql": `SELECT 1;`})
	holding := make(chan struct{})
	aDone := make(chan error, 1)
	go func() {
		_, err := Apply(ctxT(), pool, Options{Schema: schema, FS: one, Tolerant: true,
			Between: func(context.Context) error { close(holding); time.Sleep(1500 * time.Millisecond); return nil }})
		aDone <- err
	}()
	<-holding
	if _, err := Apply(ctxT(), pool, Options{Schema: schema, FS: one}); err != nil {
		t.Fatalf("waiting replica failed instead of waiting: %v", err)
	}
	if err := <-aDone; err != nil {
		t.Fatal(err)
	}
}
