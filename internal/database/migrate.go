package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// MigrateUp applies all pending migrations from the migrations directory.
// If plugin is non-empty, only that plugin's migrations are applied.
// Uses advisory locks to prevent concurrent runs, records SHA-256 checksums,
// and detects non-transactional statements (CREATE INDEX CONCURRENTLY) to
// run them outside a transaction.
// Returns the count of migrations applied.
func MigrateUp(ctx context.Context, cfg *config.Config, plugin string) (int, error) {
	if err := validatePluginArg(plugin); err != nil {
		return 0, err
	}
	if err := ensureSchemaVersions(ctx, cfg); err != nil {
		return 0, fmt.Errorf("ensure schema_versions: %w", err)
	}
	if err := ensureMigrationsTable(ctx, cfg); err != nil {
		return 0, fmt.Errorf("ensure migrations table: %w", err)
	}

	// Acquire advisory lock to prevent concurrent migrations.
	if err := acquireAdvisoryLock(ctx, cfg); err != nil {
		return 0, err
	}
	defer releaseAdvisoryLock(ctx, cfg) //nolint:errcheck

	dir := migrationsDir(cfg, plugin)
	files, err := scanMigrations(dir)
	if err != nil {
		return 0, err
	}
	if len(files) == 0 {
		return 0, nil
	}

	// Upgrade legacy ledger rows (prefix ids / nested 'up.sql' collision)
	// before computing the applied set, so old boxes neither re-run applied
	// migrations nor keep skipping never-applied ones.
	if err := upgradeLedger(ctx, cfg, files); err != nil {
		return 0, fmt.Errorf("upgrade migration ledger: %w", err)
	}

	applied, err := appliedMigrations(ctx, cfg)
	if err != nil {
		return 0, fmt.Errorf("check applied migrations: %w", err)
	}

	pending := pendingMigrationFiles(files, applied)

	// Refuse before applying anything if a pending migration's ALTER TABLE
	// target won't exist by the time it runs (see migrate_prereq.go): a
	// clear, actionable refusal beats a mid-batch "relation does not exist"
	// on a fresh database. Skipped entirely when nothing is pending, so a
	// fully-applied batch never re-evaluates this on every run.
	if missing, prereqErr := checkAlterPrerequisites(ctx, cfg, pending); prereqErr != nil {
		return 0, fmt.Errorf("check migration prerequisites: %w", prereqErr)
	} else if len(missing) > 0 {
		return 0, prerequisiteError(missing)
	}

	count := 0
	for _, f := range pending {
		name := migrationKey(f)
		if err := validateMigrationName(name); err != nil {
			return count, err
		}

		data, readErr := os.ReadFile(f)
		if readErr != nil {
			return count, fmt.Errorf("read migration %s: %w", name, readErr)
		}

		// Compute checksum for the ops table.
		checksum, _ := checksumBytes(data)
		if !sha256HexRegex.MatchString(checksum) {
			return count, fmt.Errorf("unexpected checksum format for %s", name)
		}
		migrationID := extractMigrationID(f)
		if err := validateMigrationName(migrationID); err != nil {
			return count, fmt.Errorf("migration id from %s: %w", name, err)
		}

		legacyRecord, opsRecord := migrationRecordSQL(migrationID, name, checksum)

		sqlContent := string(data)

		if isNonTransactional(sqlContent) {
			// Run non-transactional migrations outside a transaction.
			if err := pipeSQLToContainer(ctx, cfg, sqlContent); err != nil {
				return count, fmt.Errorf("migration %s: %w: %v", name, errs.ErrMigrationFailed, err)
			}
			// Record both tables atomically in a separate transaction so that
			// a failure on either INSERT leaves no orphan row in the other table.
			recordSQL := "BEGIN;\n" + legacyRecord + "\n" + opsRecord + "\nCOMMIT;\n"
			if err := pipeSQLToContainer(ctx, cfg, recordSQL); err != nil {
				return count, fmt.Errorf("record migration %s: %w", name, err)
			}
		} else {
			// Dry-run parse/plan validation before the real, irreversible apply
			// (msg-2026-07-02-nself-migration-ledger-pk-bug.md secondary finding):
			// runs the exact statements inside BEGIN;...ROLLBACK; first, so an
			// invalid construct like ADD CONSTRAINT IF NOT EXISTS surfaces here,
			// not mid-batch during the real apply below.
			if err := validateMigrationSQL(ctx, cfg, sqlContent); err != nil {
				return count, fmt.Errorf("migration %s: %w", name, err)
			}

			// DEP-02: Set lock_timeout and statement_timeout at session start to prevent
			// long-blocking schema changes from stalling production deployments.
			// lock_timeout=5s aborts if the migration cannot acquire the lock quickly.
			// statement_timeout defaults to 60s but a migration with a large data
			// backfill can override it via `-- nself:statement-timeout=...`.
			stmtTimeout := statementTimeoutFor(sqlContent)
			txSQL := "BEGIN;\n" +
				"SET LOCAL lock_timeout = '5s';\n" +
				fmt.Sprintf("SET LOCAL statement_timeout = '%s';\n", stmtTimeout) +
				sqlContent + "\n" + legacyRecord + "\n" + opsRecord + "\nCOMMIT;\n"
			if err := pipeSQLToContainer(ctx, cfg, txSQL); err != nil {
				return count, fmt.Errorf("migration %s: %w: %v", name, errs.ErrMigrationFailed, err)
			}
		}
		count++
	}
	return count, nil
}

// MigrateDown reverts the most recently applied migration.
// It looks for a corresponding .down.sql file next to the original migration.
func MigrateDown(ctx context.Context, cfg *config.Config) error {
	if err := ensureSchemaVersions(ctx, cfg); err != nil {
		return fmt.Errorf("ensure schema_versions: %w", err)
	}

	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}

	// Find the most recent migration.
	out, err := querySQL(ctx, cfg, db, "SELECT name FROM np_common.schema_versions ORDER BY applied_at DESC LIMIT 1")
	if err != nil {
		return fmt.Errorf("query latest migration: %w", err)
	}
	if out == "" {
		return fmt.Errorf("no migrations to revert")
	}
	name := strings.TrimSpace(out)
	if err := validateMigrationName(name); err != nil {
		return fmt.Errorf("migration name from schema_versions: %w", err)
	}

	// Derive the down file path from the ledger key:
	//   flat   "foo.sql"          -> <dir>/foo.down.sql
	//   nested "20260701_foo"     -> <dir>/20260701_foo/down.sql
	var downPath string
	if strings.HasSuffix(name, ".sql") {
		downPath = filepath.Join(migrationsDir(cfg, ""), strings.TrimSuffix(name, ".sql")+".down.sql")
	} else {
		downPath = filepath.Join(migrationsDir(cfg, ""), name, "down.sql")
	}

	data, readErr := os.ReadFile(downPath)
	if readErr != nil {
		return fmt.Errorf("down migration not found: %s: %w", downPath, readErr)
	}

	remove := fmt.Sprintf("DELETE FROM np_common.schema_versions WHERE name = '%s';",
		strings.ReplaceAll(name, "'", "''"))
	removeOps := fmt.Sprintf("DELETE FROM nself_ops.migrations WHERE name = '%s';",
		strings.ReplaceAll(name, "'", "''"))

	// Both deletes run inside the same transaction as the down SQL so that
	// np_common.schema_versions and nself_ops.migrations are always in sync:
	// either both rows are removed or neither is (rollback on failure).
	txSQL := "BEGIN;\n" + string(data) + "\n" + remove + "\n" + removeOps + "\nCOMMIT;\n"

	if err := pipeSQLToContainer(ctx, cfg, txSQL); err != nil {
		return fmt.Errorf("revert %s: %w: %v", name, errs.ErrMigrationFailed, err)
	}
	return nil
}
