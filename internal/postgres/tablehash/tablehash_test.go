package tablehash

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeDB records every statement and serves canned rows.
type fakeDB struct {
	execs     []string
	queries   []string
	settings  map[string]string
	failQuery bool
	encoding  string // server_encoding served; "" means UTF8
}

type fakeRow struct {
	vals []any
	err  error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i, d := range dest {
		switch p := d.(type) {
		case *string:
			*p = r.vals[i].(string)
		case *int64:
			*p = r.vals[i].(int64)
		}
	}
	return nil
}

func (f *fakeDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.execs = append(f.execs, sql+" "+args[0].(string)+"="+args[1].(string))
	f.settings[args[0].(string)] = args[1].(string)
	return pgconn.CommandTag{}, nil
}

func (f *fakeDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	if strings.HasPrefix(sql, "SELECT current_setting") {
		if args[0] == "server_encoding" {
			if f.encoding != "" {
				return fakeRow{vals: []any{f.encoding}}
			}
			return fakeRow{vals: []any{"UTF8"}}
		}
		return fakeRow{vals: []any{"orig-" + args[0].(string)}}
	}
	f.queries = append(f.queries, sql)
	if f.failQuery {
		return fakeRow{err: errors.New("relation does not exist")}
	}
	return fakeRow{vals: []any{int64(3), "42"}}
}

func TestSnapshotQuotesValidatesAndRestores(t *testing.T) {
	db := &fakeDB{settings: map[string]string{}}
	res, bad, err := Snapshot(context.Background(), db, []Table{
		{"public", "notes"}, {"public", "bad-name"}, {"Pub lic", "x"}, {"public", "Mixed_Case"},
		{"public", `x"; select 1; --`}, {"public", ""}, {"9s", "t"}, {"public", strings.Repeat("a", 64)}, {"a", "b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 6 {
		t.Fatalf("unverifiable = %v, want 6", bad)
	}
	for _, u := range bad {
		if u.Reason == "" {
			t.Errorf("%v has no reason", u.Table)
		}
	}
	if len(res) != 3 || res[0].Table != (Table{"a", "b"}) || res[1].Table != (Table{"public", "Mixed_Case"}) || res[2].Table != (Table{"public", "notes"}) {
		t.Fatalf("results not sorted by schema, name: %+v", res)
	}
	if len(db.queries) != 3 {
		t.Fatalf("queried %d tables, want 3 (unverifiable names must never be queried): %v", len(db.queries), db.queries)
	}
	if !strings.Contains(db.queries[1], `FROM "public"."Mixed_Case" AS t`) {
		t.Errorf("identifiers must be quoted: %s", db.queries[1])
	}
	for _, q := range db.queries {
		for _, frag := range []string{"count(*)", "hashtextextended(t::text, 0)::numeric", "coalesce(", ", 0)::text"} {
			if !strings.Contains(q, frag) {
				t.Errorf("query lacks %q: %s", frag, q)
			}
		}
		if strings.Contains(q, "bad-name") || strings.Contains(q, "select 1") {
			t.Errorf("unsafe name reached SQL: %s", q)
		}
	}
	// Settings: set to the fixed values first, then restored to the originals.
	for _, s := range Settings {
		if got, want := db.settings[s[0]], "orig-"+s[0]; got != want {
			t.Errorf("setting %s ends as %q, want restored %q", s[0], got, want)
		}
	}
	if len(db.execs) != 2*len(Settings) {
		t.Fatalf("set_config calls = %d, want %d", len(db.execs), 2*len(Settings))
	}
	for i, s := range Settings {
		if got := db.execs[i]; !strings.HasSuffix(got, s[0]+"="+s[1]) {
			t.Errorf("exec %d = %q, want fixed setting %s=%s", i, got, s[0], s[1])
		}
	}
	fixed := map[string]string{}
	for _, s := range Settings {
		fixed[s[0]] = s[1]
	}
	for k, v := range map[string]string{"TimeZone": "UTC", "DateStyle": "ISO", "IntervalStyle": "postgres",
		"extra_float_digits": "3", "bytea_output": "hex", "lc_monetary": "C", "statement_timeout": "0",
		"row_security": "off", "search_path": "pg_catalog"} {
		if fixed[k] != v {
			t.Errorf("Settings[%s] = %q, want %q", k, fixed[k], v)
		}
	}
}

func TestSnapshotOnlyUnverifiableRunsNoSQL(t *testing.T) {
	db := &fakeDB{settings: map[string]string{}}
	res, bad, err := Snapshot(context.Background(), db, []Table{{"public", "bad-name"}})
	if err != nil || len(res) != 0 || len(bad) != 1 {
		t.Fatalf("res=%v bad=%v err=%v", res, bad, err)
	}
	if len(db.execs)+len(db.queries) != 0 {
		t.Error("no SQL may run when nothing is verifiable")
	}
}

func TestSnapshotQueryErrorRestoresSettings(t *testing.T) {
	db := &fakeDB{settings: map[string]string{}, failQuery: true}
	_, _, err := Snapshot(context.Background(), db, []Table{{"public", "gone"}})
	if err == nil || !strings.Contains(err.Error(), "public.gone") {
		t.Fatalf("want error naming the table, got %v", err)
	}
	if db.settings["TimeZone"] != "orig-TimeZone" {
		t.Error("settings must be restored after an error")
	}
}

func TestSnapshotRefusesNonUTF8(t *testing.T) {
	db := &fakeDB{settings: map[string]string{}, encoding: "LATIN1"}
	_, _, err := Snapshot(context.Background(), db, []Table{{"public", "t"}})
	if !errors.Is(err, ErrNotUTF8) || !strings.Contains(err.Error(), "LATIN1") {
		t.Fatalf("want ErrNotUTF8 naming LATIN1, got %v", err)
	}
	if len(db.execs)+len(db.queries) != 0 {
		t.Error("a non-UTF8 database must be refused before any setting or query runs")
	}
}
