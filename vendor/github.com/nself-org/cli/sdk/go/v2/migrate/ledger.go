package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// file is one migration file read from the source.
type file struct {
	name string
	sql  string
	sum  string // sha256 hex of the raw bytes
}

// listNames returns the migration file names in lexical order: *.sql entries
// of the root directory, excluding *.down.sql, directories and anything else.
func (p *plan) listNames() ([]string, error) {
	entries, err := fs.ReadDir(p.fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("migrate: list migrations: %w", err)
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".sql") || strings.HasSuffix(n, ".down.sql") {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

// readFiles reads every migration file up front, so an unreadable file stops
// the boot before anything runs and the whole run sees one snapshot.
func (p *plan) readFiles() ([]file, error) {
	names, err := p.listNames()
	if err != nil {
		return nil, err
	}
	files := make([]file, 0, len(names))
	for _, n := range names {
		raw, err := fs.ReadFile(p.fsys, n)
		if err != nil {
			return nil, &FileError{File: n, Err: err}
		}
		sum := sha256.Sum256(raw)
		files = append(files, file{name: n, sql: string(raw), sum: hex.EncodeToString(sum[:])})
	}
	return files, nil
}

// ledgerTable is the quoted, qualified ledger table name.
func (p *plan) ledgerTable() string { return p.schema + ".schema_migrations" }

// querier is the read side shared by *pgx.Conn and *pgxpool.Pool.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// execer is the write side shared by *pgx.Conn and pgx.Tx.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// ledgerExists reports whether the ledger table exists, without creating it.
func (p *plan) ledgerExists(ctx context.Context, q querier) (bool, error) {
	var ok bool
	if err := q.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, p.ledgerTable()).Scan(&ok); err != nil {
		return false, fmt.Errorf("migrate: check ledger %s: %w", p.ledgerTable(), err)
	}
	return ok, nil
}

// ledgerHasChecksum reports whether the ledger has a checksum column. It
// reads pg_attribute, which (unlike information_schema) is exact for roles
// that hold no privileges on the table.
func (p *plan) ledgerHasChecksum(ctx context.Context, q querier) (bool, error) {
	var ok bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_attribute
		WHERE attrelid = to_regclass($1) AND attname = 'checksum' AND NOT attisdropped)`,
		p.ledgerTable()).Scan(&ok); err != nil {
		return false, fmt.Errorf("migrate: inspect ledger %s: %w", p.ledgerTable(), err)
	}
	return ok, nil
}

// schemaExists reports whether the plugin schema exists.
func (p *plan) schemaExists(ctx context.Context, q querier) (bool, error) {
	var ok bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)`,
		p.opts.Schema).Scan(&ok); err != nil {
		return false, fmt.Errorf("migrate: check schema %s: %w", p.opts.Schema, err)
	}
	return ok, nil
}

// schemaHasObjects reports whether the schema already holds tables, views,
// sequences, indexes, functions or enum/domain types.
func (p *plan) schemaHasObjects(ctx context.Context, q querier) (bool, error) {
	var ok bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = $1)
		OR EXISTS (SELECT 1 FROM pg_proc f JOIN pg_namespace n ON n.oid = f.pronamespace WHERE n.nspname = $1)
		OR EXISTS (SELECT 1 FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace
			WHERE n.nspname = $1 AND t.typtype IN ('e', 'd'))`, p.opts.Schema).Scan(&ok); err != nil {
		return false, fmt.Errorf("migrate: inspect schema %s: %w", p.opts.Schema, err)
	}
	return ok, nil
}

// createLedger creates the schema (only when absent: a plugin role may lack
// CREATE on the database, and the CLI normally creates the schema first) and
// the ledger table.
func (p *plan) createLedger(ctx context.Context, ex execer, haveSchema bool) error {
	if !haveSchema {
		if _, err := ex.Exec(ctx, `CREATE SCHEMA `+p.schema); err != nil {
			return fmt.Errorf("migrate: create schema %s: %w", p.opts.Schema, err)
		}
	}
	if _, err := ex.Exec(ctx, `CREATE TABLE `+p.ledgerTable()+` (
		filename   text PRIMARY KEY,
		checksum   text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("migrate: create ledger %s: %w", p.ledgerTable(), err)
	}
	return nil
}

// ensureLedger makes sure a usable ledger exists, under the advisory lock. It
// refuses to guess: a schema that already has objects but no ledger was set up
// by hand, and replaying its files would fail or corrupt data, so the plugin
// must call Baseline once. Likewise a ledger without a checksum column.
func (p *plan) ensureLedger(ctx context.Context, conn *pgx.Conn) error {
	exists, err := p.ledgerExists(ctx, conn)
	if err != nil {
		return err
	}
	if exists {
		ok, err := p.ledgerHasChecksum(ctx, conn)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: %s has no checksum column (a ledger from before contract "+
				"plugin.boot-migrations v1); call migrate.Baseline once to add and backfill it",
				ErrLegacyLedger, p.ledgerTable())
		}
		return nil
	}
	haveSchema, err := p.schemaExists(ctx, conn)
	if err != nil {
		return err
	}
	if haveSchema {
		objs, err := p.schemaHasObjects(ctx, conn)
		if err != nil {
			return err
		}
		if objs {
			return fmt.Errorf("%w: schema %s already has objects but %s does not exist, so Apply "+
				"would replay every file over them; if the schema was set up by hand, call "+
				"migrate.Baseline once to record the existing files without running them",
				ErrUnledgered, p.opts.Schema, p.ledgerTable())
		}
	}
	return p.createLedger(ctx, conn, haveSchema)
}

// loadLedger returns filename -> recorded checksum.
func (p *plan) loadLedger(ctx context.Context, conn *pgx.Conn) (map[string]string, error) {
	rows, err := conn.Query(ctx, `SELECT filename, checksum FROM `+p.ledgerTable())
	if err != nil {
		return nil, fmt.Errorf("migrate: read ledger: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, sum string
		if err := rows.Scan(&name, &sum); err != nil {
			return nil, fmt.Errorf("migrate: read ledger: %w", err)
		}
		out[name] = sum
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrate: read ledger: %w", err)
	}
	return out, nil
}

// applyFile runs one file and records it in one transaction. On any error
// the transaction rolls back: no effect and no ledger row survive.
func (p *plan) applyFile(ctx context.Context, conn *pgx.Conn, f file) (err error) {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() {
		if err != nil {
			rctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tx.Rollback(rctx)
		}
	}()
	if err = p.pinSettings(ctx, tx); err != nil {
		return err
	}
	if strings.TrimSpace(f.sql) != "" {
		if _, err = tx.Exec(ctx, f.sql); err != nil {
			return err
		}
		// Defence in depth behind findTxControl: if the file still ended the
		// transaction, what it did is committed and cannot be rolled back.
		if conn.PgConn().TxStatus() != 'T' {
			return fmt.Errorf("%w (the file ended its own transaction, so its effects are committed without a ledger row)", ErrTxControl)
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO `+p.ledgerTable()+` (filename, checksum) VALUES ($1, $2)`,
		f.name, f.sum); err != nil {
		return fmt.Errorf("record in ledger: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// pinSettings sets lock_timeout, statement_timeout and search_path for the
// current transaction only: set_config(..., true) is SET LOCAL.
func (p *plan) pinSettings(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `SELECT set_config('lock_timeout', $1, true),
		set_config('statement_timeout', $2, true), set_config('search_path', $3, true)`,
		fmt.Sprintf("%dms", p.opts.LockTimeout.Milliseconds()),
		fmt.Sprintf("%dms", p.opts.StatementTimeout.Milliseconds()), p.path); err != nil {
		return fmt.Errorf("pin session settings: %w", err)
	}
	return nil
}
