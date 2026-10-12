package exporter

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/portable"
	"github.com/nself-org/cli/internal/postgres/tablehash"
)

type snapshot struct {
	conn    *pgx.Conn
	tx      pgx.Tx
	id      string
	info    portable.PostgresInfo
	schemas []string
	tables  []tablehash.Table
	bytes   uint64
}

// Purpose: hold one read-only repeatable-read snapshot for all database reads.
// Inputs: host DSN via config. Outputs: live snapshot. Constraints: safe errors.
func openSnapshot(ctx context.Context, cfg *config.Config, requested []string) (*snapshot, error) {
	c, err := pgx.Connect(ctx, cfg.HostDatabaseURL())
	if err != nil {
		return nil, safeFailure("connect database")
	}
	s := &snapshot{conn: c}
	s.tx, err = c.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err == nil {
		var version int
		err = s.tx.QueryRow(ctx, "SELECT pg_export_snapshot(), current_setting('server_version'), current_setting('server_version_num')::int").Scan(&s.id, &s.info.ServerVersion, &version)
		s.info.Major = version / 10000
	}
	if err == nil {
		err = s.catalog(ctx, requested)
	}
	if err != nil {
		s.close(ctx)
		return nil, safeFailure("read database catalog")
	}
	return s, nil
}

func (s *snapshot) close(ctx context.Context) {
	if s.tx != nil {
		_ = s.tx.Rollback(ctx)
	}
	_ = s.conn.Close(ctx)
}

// Purpose: select exportable schemas/tables and estimate their disk size.
// Inputs: explicit schemas or all user schemas. Outputs: sorted catalog.
// Constraints: plugin and platform data cannot be selected explicitly.
func (s *snapshot) catalog(ctx context.Context, requested []string) error {
	var err error
	s.schemas, err = selectedSchemas(ctx, s.tx, requested)
	if err != nil {
		return err
	}
	all, err := tablehash.ListTables(ctx, s.tx, s.schemas)
	if err != nil {
		return err
	}
	for _, t := range all {
		if !strings.HasPrefix(t.Name, "np_") {
			s.tables = append(s.tables, t)
		}
	}
	for _, t := range s.tables {
		var size uint64
		if err = s.tx.QueryRow(ctx, "SELECT pg_total_relation_size($1::regclass)", pgx.Identifier{t.Schema, t.Name}.Sanitize()).Scan(&size); err != nil {
			return err
		}
		if size > ^uint64(0)-s.bytes {
			return safeFailure("estimate database")
		}
		s.bytes += size
	}
	return nil
}

// Purpose: resolve requested schemas against the snapshot catalog.
// Inputs: explicit names or all schemas. Outputs: sorted user schema names.
// Constraints: missing and platform/plugin schemas are refused, never widened.
func selectedSchemas(ctx context.Context, tx pgx.Tx, requested []string) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT nspname::text FROM pg_catalog.pg_namespace ORDER BY nspname COLLATE "C"`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var schemas []string
	wanted := map[string]bool{}
	for _, name := range requested {
		wanted[name] = true
	}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, err
		}
		if !excludedSchema(name) && (len(requested) == 0 || wanted[name]) {
			schemas = append(schemas, name)
			delete(wanted, name)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(wanted) > 0 {
		return nil, safeFailure("select schemas")
	}
	return schemas, nil
}

func excludedSchema(name string) bool {
	return strings.HasPrefix(name, "pg_") || strings.HasPrefix(name, "np_") || name == "information_schema" || name == "nself_ops" || name == "hdb_catalog" || name == "auth"
}

// Purpose: hash and dump under the exported snapshot. Inputs: writer.
// Outputs: database members and table records. Constraints: no partial hash success.
func (s *snapshot) write(ctx context.Context, cfg *config.Config, w *portable.Writer, m *portable.Manifest) error {
	results, bad, err := tablehash.Snapshot(ctx, s.tx, s.tables)
	if err != nil || len(bad) > 0 {
		return safeFailure("hash database")
	}
	for _, r := range results {
		pk, err := s.primaryKey(ctx, r.Table)
		if err != nil {
			return safeFailure("read primary keys")
		}
		m.DB.Tables = append(m.DB.Tables, portable.Table{Schema: r.Table.Schema, Name: r.Table.Name, Rows: r.Rows, Hash: r.Hash, PK: pk})
		m.SourceCounts = append(m.SourceCounts, portable.SourceCount{Kind: "table", ID: r.Table.String(), Count: r.Rows, Method: "snapshot count(*)"})
	}
	if err = dump(ctx, cfg, w, s.id, s.schemas); err != nil {
		return err
	}
	return nil
}

func (s *snapshot) primaryKey(ctx context.Context, t tablehash.Table) ([]string, error) {
	var pk []string
	err := s.tx.QueryRow(ctx, `SELECT coalesce(array_agg(a.attname::text ORDER BY k.ord), ARRAY[]::text[])
FROM pg_catalog.pg_index i CROSS JOIN LATERAL unnest(i.indkey) WITH ORDINALITY k(num,ord)
JOIN pg_catalog.pg_attribute a ON a.attrelid=i.indrelid AND a.attnum=k.num
WHERE i.indrelid=$1::regclass AND i.indisprimary`, pgx.Identifier{t.Schema, t.Name}.Sanitize()).Scan(&pk)
	return pk, err
}
