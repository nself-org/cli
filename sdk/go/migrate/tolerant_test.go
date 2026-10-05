package migrate

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

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

// A tolerant boot that fails after applying 004 but before the deferred 002
// and 003 leaves an applied file ahead of pending ones. The retry must work:
// the out-of-order refusal is strict-mode only.
func TestApplyTolerantRetryAfterFailedBoot(t *testing.T) {
	pool := newPool(t)
	schema := schemaFor(t, pool)
	files := sqlFS(map[string]string{
		"001.sql": `CREATE TABLE a(n int);`,
		"002.sql": `ALTER TABLE go_table ADD COLUMN x int;`,
		"004.sql": `CREATE TABLE b(n int);`,
	})
	opts := Options{Schema: schema, FS: files, Tolerant: true,
		Between: func(context.Context) error { return errors.New("go migration failed") }}
	if _, err := Apply(ctxT(), pool, opts); err == nil {
		t.Fatal("first boot must fail")
	}
	opts.Between = func(ctx context.Context) error {
		_, err := pool.Exec(ctx, `CREATE TABLE "`+schema+`".go_table(id serial)`)
		return err
	}
	res, err := Apply(ctxT(), pool, opts)
	if err != nil || !reflect.DeepEqual(res.Applied, []string{"002.sql"}) {
		t.Fatalf("retry: %v %v", res, err)
	}
}
