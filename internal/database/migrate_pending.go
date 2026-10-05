package database

// Purpose: PendingMigrations, the read-only planner behind `nself db migrate
// up --dry-run` without --migration-dir (P7-PROD-84).
// Inputs: a *config.Config and an optional plugin name (selects the directory).
// Outputs: the ledger keys MigrateUp would apply, in apply order.
// Constraints: issues SELECT statements only. It never runs ensure*, never
// upgradeLedger and never creates a table: a missing ledger means "nothing
// applied". The old implementation called both ensure* helpers and
// upgradeLedger, so a "dry run" created tables and rewrote ledger rows.
// MigrateUp keeps those writes; only the preview is read-only.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
)

// PendingMigrations returns the migration keys that have not yet been applied,
// without writing anything. The pending set matches what MigrateUp would apply
// after its ledger upgrade: a legacy 'up.sql' row (nested layout, written
// before keys were unique) is resolved in memory with the same resolver.
func PendingMigrations(ctx context.Context, cfg *config.Config, plugin string) ([]string, error) {
	files, err := scanMigrations(migrationsDir(cfg, plugin))
	if err != nil {
		return nil, err
	}
	applied := map[string]time.Time{}
	if ok, existsErr := ledgerTableExists(ctx, cfg, "np_common.schema_versions"); existsErr != nil {
		return nil, existsErr
	} else if ok {
		if applied, err = appliedMigrations(ctx, cfg); err != nil {
			return nil, fmt.Errorf("check applied migrations: %w", err)
		}
		if err = resolveLegacyAppliedRow(ctx, cfg, files, applied); err != nil {
			return nil, err
		}
	}
	var pending []string
	for _, f := range pendingMigrationFiles(files, applied) {
		pending = append(pending, migrationKey(f))
	}
	return pending, nil
}

// resolveLegacyAppliedRow rewrites applied in memory the way upgradeLedger
// rewrites the table: the legacy 'up.sql' entry becomes the key of the nested
// migration that actually ran. Reads only (the optional ops ids come from a
// SELECT guarded by to_regclass).
func resolveLegacyAppliedRow(ctx context.Context, cfg *config.Config, files []string, applied map[string]time.Time) error {
	at, ok := applied["up.sql"]
	if !ok {
		return nil
	}
	var opsIDs []string
	if exists, err := ledgerTableExists(ctx, cfg, "nself_ops.migrations"); err != nil {
		return err
	} else if exists {
		out, err := ledgerSelect(ctx, cfg, "SELECT id FROM nself_ops.migrations WHERE name = 'up.sql' ORDER BY id")
		if err != nil {
			return fmt.Errorf("ledger legacy ops rows: %w", err)
		}
		for _, line := range strings.Split(out, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				opsIDs = append(opsIDs, line)
			}
		}
	}
	key := resolveLegacyNestedKey(files, opsIDs)
	if key == "" {
		return nil
	}
	if err := validateMigrationName(key); err != nil {
		return fmt.Errorf("ledger upgrade key: %w", err)
	}
	delete(applied, "up.sql")
	if _, has := applied[key]; !has {
		applied[key] = at
	}
	return nil
}
