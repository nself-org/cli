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
			waitNoAdvisoryLocks(t, pool)
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

// waitNoAdvisoryLocks polls until the server has released every advisory lock.
// closeConn sends Terminate and returns; the backend exits (and drops its
// session lock) a moment later, so an immediate count would race it.
func waitNoAdvisoryLocks(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		n := count(t, pool, `SELECT count(*) FROM pg_locks WHERE locktype='advisory'`)
		if n == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d advisory locks still held 10s after Apply returned", n)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
