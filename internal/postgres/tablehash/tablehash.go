// Package tablehash computes a per-table row count and an order-independent
// content hash, the single definition of "zero mismatch" shared by the
// importer, the exporter, the round trip and the image switch
// (EPIC D8, contract:cli.portable-export v1, P7-ADOPT-23).
//
// Definition: count = count(*); hash = sum(hashtextextended(t::text, 0)::numeric)
// as a decimal string over every row t of the table, "0" for an empty table.
// The sum makes the hash independent of row order; t::text is the row's text
// form, so the session settings below are fixed first. They make the text form
// identical across servers, session defaults and Postgres 14-17.
//
// Layering: L1; imports only pgx and the standard library.
package tablehash

import (
	"context"
	"fmt"
	"regexp"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Settings are the session settings Snapshot applies while it hashes. Order
// is fixed. Each value is restored afterwards on a best-effort basis.
var Settings = [][2]string{
	{"TimeZone", "UTC"},
	{"DateStyle", "ISO"},
	{"IntervalStyle", "postgres"},
	{"extra_float_digits", "3"},
	{"bytea_output", "hex"},
	{"lc_monetary", "C"},
	{"statement_timeout", "0"},
}

// IdentifierPattern is the accepted shape of a schema or table name part.
const IdentifierPattern = `^[A-Za-z_][A-Za-z0-9_]{0,62}$`

// identRe is the only identifier shape that is ever queried.
var identRe = regexp.MustCompile(IdentifierPattern)

// Table names one table.
type Table struct {
	Schema string
	Name   string
}

func (t Table) String() string { return t.Schema + "." + t.Name }

// Result is the count and hash of one table.
type Result struct {
	Table Table
	Rows  int64
	// Hash is a decimal string; "0" for an empty table.
	Hash string
}

// Unverifiable is a table that was not queried, with the reason.
type Unverifiable struct {
	Table  Table
	Reason string
}

// DB is the part of *pgx.Conn and pgx.Tx that Snapshot uses. Pass a pgx.Tx
// opened with REPEATABLE READ to hash several tables against one snapshot.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Snapshot hashes tables with the fixed Settings.
//
// Purpose: count and hash every listed table the same way on every server.
// Inputs: tables in any order; names whose schema or table part does not match
// IdentifierPattern are never queried and come back as Unverifiable.
// Outputs: results and unverifiable tables, both sorted by schema then name
// (byte order); the error is the first failed query (a missing table is an
// error, not an Unverifiable).
// Constraints: identifiers are quoted with pgx.Identifier after validation.
// The session settings are restored before return.
func Snapshot(ctx context.Context, db DB, tables []Table) ([]Result, []Unverifiable, error) {
	sorted := append([]Table(nil), tables...)
	sort.Slice(sorted, func(i, j int) bool { return less(sorted[i], sorted[j]) })

	var bad []Unverifiable
	var ok []Table
	for _, t := range sorted {
		if !identRe.MatchString(t.Schema) || !identRe.MatchString(t.Name) {
			bad = append(bad, Unverifiable{Table: t, Reason: "name outside " + IdentifierPattern})
			continue
		}
		ok = append(ok, t)
	}
	if len(ok) == 0 {
		return nil, bad, nil
	}
	restore, err := applySettings(ctx, db)
	if err != nil {
		return nil, nil, err
	}
	defer restore()

	results := make([]Result, 0, len(ok))
	for _, t := range ok {
		q := "SELECT count(*), coalesce(sum(hashtextextended(t::text, 0)::numeric), 0)::text FROM " +
			pgx.Identifier{t.Schema, t.Name}.Sanitize() + " AS t"
		r := Result{Table: t}
		if err := db.QueryRow(ctx, q).Scan(&r.Rows, &r.Hash); err != nil {
			return nil, nil, fmt.Errorf("tablehash: %s: %w", t, err)
		}
		results = append(results, r)
	}
	return results, bad, nil
}

func less(a, b Table) bool {
	return a.Schema < b.Schema || (a.Schema == b.Schema && a.Name < b.Name)
}

// applySettings sets every setting for the session and returns a function that
// puts the previous values back.
func applySettings(ctx context.Context, db DB) (func(), error) {
	old := make([]string, len(Settings))
	for i, s := range Settings {
		if err := db.QueryRow(ctx, "SELECT current_setting($1)", s[0]).Scan(&old[i]); err != nil {
			return func() {}, fmt.Errorf("tablehash: read %s: %w", s[0], err)
		}
	}
	restore := func() {
		for i, s := range Settings {
			_, _ = db.Exec(ctx, "SELECT set_config($1, $2, false)", s[0], old[i])
		}
	}
	for _, s := range Settings {
		if _, err := db.Exec(ctx, "SELECT set_config($1, $2, false)", s[0], s[1]); err != nil {
			restore()
			return func() {}, fmt.Errorf("tablehash: set %s: %w", s[0], err)
		}
	}
	return restore, nil
}

// Querier is the part of *pgx.Conn and pgx.Tx that ListTables uses.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// ListTables returns the ordinary and partitioned tables of the given schemas
// from pg_catalog, sorted by schema then name in byte order. Partitions are
// left out: the parent's hash already covers their rows.
func ListTables(ctx context.Context, db Querier, schemas []string) ([]Table, error) {
	rows, err := db.Query(ctx, `SELECT n.nspname::text, c.relname::text
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r','p') AND NOT c.relispartition AND n.nspname = ANY($1)
ORDER BY n.nspname COLLATE "C", c.relname COLLATE "C"`, schemas)
	if err != nil {
		return nil, fmt.Errorf("tablehash: list tables: %w", err)
	}
	defer rows.Close()
	var out []Table
	for rows.Next() {
		var t Table
		if err := rows.Scan(&t.Schema, &t.Name); err != nil {
			return nil, fmt.Errorf("tablehash: list tables: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tablehash: list tables: %w", err)
	}
	return out, nil
}
