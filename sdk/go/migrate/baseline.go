package migrate

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Baseline adopts an install whose schema was set up before this helper: it
// records every current migration file in the ledger, with its checksum,
// without running any of them. Call it once, deliberately (a one-off command
// or a guarded boot step), when Apply returned ErrUnledgered or ErrLegacyLedger.
//
//   - No ledger: the schema (if absent) and the ledger are created, then every
//     file is recorded.
//   - A legacy (filename, applied_at) ledger such as claw's: a checksum column
//     is added and backfilled from the current files. A row whose file is no
//     longer on disk gets the empty checksum, and Status reports it as deleted.
//     Then every file not yet recorded is recorded.
//   - A current ledger: only files not yet recorded are added. Existing rows,
//     including drifted ones, are left alone.
//
// It runs under the same advisory lock as Apply, in one transaction. Result
// lists the files it recorded in Baselined. Tolerant and Between are ignored.
func Baseline(ctx context.Context, pool *pgxpool.Pool, opts Options) (res Result, err error) {
	p, err := newPlan(opts)
	if err != nil {
		return res, err
	}
	files, err := p.readFiles()
	if err != nil {
		return res, err
	}
	conn, err := lockConn(ctx, pool, p.lockKey)
	if err != nil {
		return res, err
	}
	defer closeConn(conn)

	tx, err := conn.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("migrate: baseline: begin: %w", err)
	}
	defer func() {
		if err != nil {
			rctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tx.Rollback(rctx)
		}
	}()
	if err = p.pinSettings(ctx, tx); err != nil {
		return res, err
	}

	exists, err := p.ledgerExists(ctx, tx)
	if err != nil {
		return res, err
	}
	if !exists {
		haveSchema, serr := p.schemaExists(ctx, tx)
		if serr != nil {
			return res, serr
		}
		if err = p.createLedger(ctx, tx, haveSchema); err != nil {
			return res, err
		}
	} else if err = p.upgradeLedger(ctx, tx, files); err != nil {
		return res, err
	}

	for _, f := range files {
		tag, ierr := tx.Exec(ctx, `INSERT INTO `+p.ledgerTable()+` (filename, checksum)
			SELECT $1::text, $2::text WHERE NOT EXISTS
			(SELECT 1 FROM `+p.ledgerTable()+` WHERE filename = $1::text)`, f.name, f.sum)
		if ierr != nil {
			return res, fmt.Errorf("migrate: baseline: record %s: %w", f.name, ierr)
		}
		if tag.RowsAffected() == 1 {
			res.Baselined = append(res.Baselined, f.name)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return res, fmt.Errorf("migrate: baseline: commit: %w", err)
	}
	return res, nil
}

// upgradeLedger adds and backfills the checksum column of a legacy ledger.
// A ledger that already has the column is left as it is.
func (p *plan) upgradeLedger(ctx context.Context, tx interface {
	querier
	execer
}, files []file) error {
	has, err := p.ledgerHasChecksum(ctx, tx)
	if err != nil || has {
		return err
	}
	if _, err := tx.Exec(ctx, `ALTER TABLE `+p.ledgerTable()+` ADD COLUMN checksum text`); err != nil {
		return fmt.Errorf("migrate: baseline: add checksum column: %w", err)
	}
	for _, f := range files {
		if _, err := tx.Exec(ctx, `UPDATE `+p.ledgerTable()+` SET checksum = $2 WHERE filename = $1`,
			f.name, f.sum); err != nil {
			return fmt.Errorf("migrate: baseline: backfill %s: %w", f.name, err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE `+p.ledgerTable()+` SET checksum = '' WHERE checksum IS NULL`); err != nil {
		return fmt.Errorf("migrate: baseline: backfill deleted files: %w", err)
	}
	if _, err := tx.Exec(ctx, `ALTER TABLE `+p.ledgerTable()+` ALTER COLUMN checksum SET NOT NULL`); err != nil {
		return fmt.Errorf("migrate: baseline: checksum not null: %w", err)
	}
	return nil
}
