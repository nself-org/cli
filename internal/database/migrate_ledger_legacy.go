package database

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// Purpose: one-time upgrade path for ledger rows written before the unique-key
// scheme existed — rewriting old date-prefix ids in nself_ops.migrations and
// resolving the legacy nested-layout 'up.sql' collision to the correct key.
// Inputs: a *config.Config, the on-disk migration file list, and (for the
// resolver) the old-style ops ids recorded alongside a legacy row.
// Outputs: idempotent SQL side effects against the ledger tables.
// Constraints: split out of migrate.go (CLI-R12) as a pure move; no behavior
// changed. Depends on migrate_sql.go (pipeSQLToContainer, querySQL) and
// migrate_ledger.go (migrationKey, validateMigrationName).

// legacyOpsIDBackfillWhere selects the nself_ops.migrations rows (alias m)
// whose id was the old date/timestamp prefix (extractMigrationID used to
// truncate at the first underscore) and can move to the new unique id (full
// name minus ".sql"). The `id <>` guard makes re-runs a no-op; the NOT EXISTS
// guard avoids PK conflicts if a new-style row already exists. Rows named
// 'up.sql' (legacy nested layout) are handled separately by
// legacyNestedRenameSQL. One predicate for the UPDATE and for the read-only
// count in ledgerNeedsUpgrade, so the two cannot drift.
const legacyOpsIDBackfillWhere = `m.name LIKE '%.sql'
  AND m.name <> 'up.sql'
  AND m.id <> left(m.name, length(m.name) - 4)
  AND NOT EXISTS (
    SELECT 1 FROM nself_ops.migrations m2
    WHERE m2.id = left(m.name, length(m.name) - 4)
  )`

// legacyOpsIDBackfillSQL upgrades those rows in place. Idempotent.
const legacyOpsIDBackfillSQL = `UPDATE nself_ops.migrations m
SET id = left(m.name, length(m.name) - 4)
WHERE ` + legacyOpsIDBackfillWhere + `;`

// legacyOpsIDCountSQL counts the rows legacyOpsIDBackfillSQL would rewrite.
const legacyOpsIDCountSQL = `SELECT count(*) FROM nself_ops.migrations m WHERE ` + legacyOpsIDBackfillWhere

// legacyUpRowCountSQL counts the legacy nested-layout ledger rows.
const legacyUpRowCountSQL = "SELECT count(*) FROM np_common.schema_versions WHERE name = 'up.sql'"

// legacyNestedRenameSQL rewrites the single legacy nested-layout ledger row
// (name = 'up.sql') to the resolved migration key. Idempotent: after the first
// run no 'up.sql' rows remain, so every statement is a no-op. The trailing
// DELETEs clean up only when the target key already exists (never loses the
// applied fact — the UPDATE runs first).
func legacyNestedRenameSQL(key string) string {
	k := strings.ReplaceAll(key, "'", "''")
	return fmt.Sprintf(`BEGIN;
UPDATE np_common.schema_versions SET name = '%s' WHERE name = 'up.sql' AND NOT EXISTS (SELECT 1 FROM np_common.schema_versions sv2 WHERE sv2.name = '%s');
DELETE FROM np_common.schema_versions WHERE name = 'up.sql';
UPDATE nself_ops.migrations SET id = '%s', name = '%s' WHERE name = 'up.sql' AND NOT EXISTS (SELECT 1 FROM nself_ops.migrations m2 WHERE m2.id = '%s');
DELETE FROM nself_ops.migrations WHERE name = 'up.sql';
COMMIT;
`, k, k, k, k, k)
}

// resolveLegacyNestedKey decides which on-disk nested migration a legacy
// 'up.sql' ledger row belongs to. opsIDs are the old prefix-style ids of
// nself_ops.migrations rows named 'up.sql' (recorded in the same transaction
// as the legacy row, so they identify the directory that actually ran).
// Falls back to the first nested migration in sorted order — under the old
// code only the lexicographically first nested migration could ever have been
// applied (all later ones were skipped by the name collision).
// Returns "" when no nested migrations exist on disk.
func resolveLegacyNestedKey(files []string, opsIDs []string) string {
	var nested []string
	for _, f := range files {
		if filepath.Base(f) == "up.sql" {
			nested = append(nested, migrationKey(f))
		}
	}
	if len(nested) == 0 {
		return ""
	}
	sort.Strings(nested)

	for _, id := range opsIDs {
		var matches []string
		for _, k := range nested {
			if k == id || strings.SplitN(k, "_", 2)[0] == id {
				matches = append(matches, k)
			}
		}
		if len(matches) == 1 {
			return matches[0]
		}
	}
	return nested[0]
}

// upgradeLedger migrates legacy ledger rows (written before the unique-key
// scheme) in place so already-deployed boxes upgrade cleanly without
// re-running applied migrations:
//  1. nself_ops.migrations rows keyed by the old date prefix get their id
//     rewritten to the full unique id (SQL-side, idempotent).
//  2. The legacy nested-layout 'up.sql' row in both ledgers is renamed to the
//     directory-derived key of the migration that actually ran.
//
// Must be called after ensureSchemaVersions + ensureMigrationsTable.
func upgradeLedger(ctx context.Context, cfg *config.Config, files []string) error {
	if err := pipeSQLToContainer(ctx, cfg, legacyOpsIDBackfillSQL); err != nil {
		return fmt.Errorf("ledger id backfill: %w", err)
	}

	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}

	out, err := querySQL(ctx, cfg, db, legacyUpRowCountSQL)
	if err != nil {
		return fmt.Errorf("ledger legacy row check: %w", err)
	}
	if strings.TrimSpace(out) == "0" || strings.TrimSpace(out) == "" {
		return nil
	}

	opsOut, err := querySQL(ctx, cfg, db, "SELECT id FROM nself_ops.migrations WHERE name = 'up.sql' ORDER BY id")
	if err != nil {
		return fmt.Errorf("ledger legacy ops rows: %w", err)
	}
	var opsIDs []string
	for _, line := range strings.Split(opsOut, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			opsIDs = append(opsIDs, line)
		}
	}

	key := resolveLegacyNestedKey(files, opsIDs)
	if key == "" {
		// No nested migrations on disk: nothing the row could collide with.
		return nil
	}
	if err := validateMigrationName(key); err != nil {
		return fmt.Errorf("ledger upgrade key: %w", err)
	}
	if err := pipeSQLToContainer(ctx, cfg, legacyNestedRenameSQL(key)); err != nil {
		return fmt.Errorf("ledger legacy row rename: %w", err)
	}
	return nil
}

// ledgerNeedsUpgrade reports, with SELECTs only, whether upgradeLedger would
// write anything: (a) an nself_ops.migrations row matches the id-backfill
// predicate, or (b) np_common.schema_versions holds the legacy 'up.sql' row and
// a nested migration on disk can take its key. A missing ledger table means
// nothing to upgrade (false, no error); nothing is ever created.
func ledgerNeedsUpgrade(ctx context.Context, cfg *config.Config, files []string) (bool, error) {
	opsExists, err := ledgerTableExists(ctx, cfg, "nself_ops.migrations")
	if err != nil {
		return false, err
	}
	if opsExists {
		n, err := ledgerCount(ctx, cfg, legacyOpsIDCountSQL)
		if err != nil {
			return false, fmt.Errorf("ledger id backfill check: %w", err)
		}
		if n > 0 {
			return true, nil
		}
	}
	// With any nested migration on disk resolveLegacyNestedKey never returns "":
	// the ops ids only choose which one, so nil is enough to ask "is there one".
	if resolveLegacyNestedKey(files, nil) == "" {
		return false, nil
	}
	svExists, err := ledgerTableExists(ctx, cfg, "np_common.schema_versions")
	if err != nil || !svExists {
		return false, err
	}
	n, err := ledgerCount(ctx, cfg, legacyUpRowCountSQL)
	if err != nil {
		return false, fmt.Errorf("ledger legacy row check: %w", err)
	}
	return n > 0, nil
}

// ledgerCount runs a read-only count(*) query; an empty answer reads as 0 and
// a non-integer answer is an error, never a silent zero.
func ledgerCount(ctx context.Context, cfg *config.Config, q string) (int, error) {
	out, err := ledgerSelect(ctx, cfg, q)
	if err != nil {
		return 0, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(out)
	if err != nil {
		return 0, fmt.Errorf("unexpected count %q: %w", out, err)
	}
	return n, nil
}

// upgradeLedgerIfNeeded runs upgradeLedger only when ledgerNeedsUpgrade says it
// would write, so a clean ledger costs reads, never writes. The ledger tables
// must already exist (ensureLedgerTables).
func upgradeLedgerIfNeeded(ctx context.Context, cfg *config.Config, files []string) error {
	need, err := ledgerNeedsUpgrade(ctx, cfg, files)
	if err != nil {
		return fmt.Errorf("check migration ledger: %w", err)
	}
	if !need {
		return nil
	}
	if err := upgradeLedger(ctx, cfg, files); err != nil {
		return fmt.Errorf("upgrade migration ledger: %w", err)
	}
	return nil
}
