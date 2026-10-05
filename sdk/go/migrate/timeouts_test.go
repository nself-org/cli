package migrate

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

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
