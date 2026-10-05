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

// ensureLedger creates the schema (only when absent: a plugin role may lack
// CREATE on the database, and the CLI normally creates the schema first) and
// the ledger table. Runs under the advisory lock, so there is no create race.
func (p *plan) ensureLedger(ctx context.Context, conn *pgx.Conn) error {
	var haveSchema bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)`,
		p.opts.Schema).Scan(&haveSchema); err != nil {
		return fmt.Errorf("migrate: check schema %s: %w", p.opts.Schema, err)
	}
	if !haveSchema {
		if _, err := conn.Exec(ctx, `CREATE SCHEMA `+p.schema); err != nil {
			return fmt.Errorf("migrate: create schema %s: %w", p.opts.Schema, err)
		}
	}
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS `+p.ledgerTable()+` (
		filename   text PRIMARY KEY,
		checksum   text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("migrate: create ledger %s: %w", p.ledgerTable(), err)
	}
	var hasChecksum bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'schema_migrations' AND column_name = 'checksum')`,
		p.opts.Schema).Scan(&hasChecksum); err != nil {
		return fmt.Errorf("migrate: inspect ledger: %w", err)
	}
	if !hasChecksum {
		return fmt.Errorf("migrate: ledger %s has no checksum column (a ledger from before contract "+
			"plugin.boot-migrations v1); add the column and backfill it before adopting this helper", p.ledgerTable())
	}
	return nil
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
	// set_config(..., true) is SET LOCAL: the settings die with the transaction.
	if _, err = tx.Exec(ctx, `SELECT set_config('lock_timeout', $1, true),
		set_config('statement_timeout', $2, true), set_config('search_path', $3, true)`,
		fmt.Sprintf("%dms", p.opts.LockTimeout.Milliseconds()),
		fmt.Sprintf("%dms", p.opts.StatementTimeout.Milliseconds()), p.path); err != nil {
		return fmt.Errorf("pin session settings: %w", err)
	}
	if strings.TrimSpace(f.sql) != "" {
		if _, err = tx.Exec(ctx, f.sql); err != nil {
			return err
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
