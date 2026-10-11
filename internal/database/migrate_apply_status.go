package database

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/nself-org/cli/internal/config"
)

// Purpose: MigrateStatus, the read-only status behind `nself db migrate status`
// (D-0378): merged on-disk/ledger state, SELECT statements only.
// Inputs: a *config.Config and an optional --migration-dir.
// Outputs: []MigrationStatus, plus a warning line on statusWarnOut when the
// ledger still holds legacy rows that up/down would upgrade.
// Constraints: split out of migrate_apply.go (300-line limit). Never runs
// ensure* or upgradeLedger; MigrateUp, MigrateUpDir and MigrateDownDir do.

// statusWarnOut receives MigrateStatus's ledger-upgrade warning (tests swap it).
var statusWarnOut io.Writer = os.Stderr

// ledgerUpgradeWarning is written once when status found legacy ledger rows.
const ledgerUpgradeWarning = "warning: the migration ledger has legacy rows (old prefix ids or a nested 'up.sql' row); " +
	"status shows them as upgraded but wrote nothing. Run 'nself db migrate up' " +
	"(with the same --migration-dir, if you used one) to upgrade the ledger."

// MigrateStatus returns the status of all known migrations (applied and pending).
// It merges on-disk migration files with the schema_versions table, so orphaned
// migrations (applied but no longer on disk) are also reported.
//
// Read-only (D-0378): it issues SELECTs only. It creates no table and rewrites
// no row; a missing ledger means nothing is applied. A legacy 'up.sql' row is
// resolved in memory the way upgradeLedger would, and when the ledger does need
// that upgrade a warning goes to statusWarnOut. up and down perform the upgrade.
//
// dir overrides the auto-detected migrations directory (the --migration-dir
// companion to MigrateUpDir, G-008): repos with non-standard layouts, e.g.
// ntask's postgres/migrations, would otherwise report "No migrations found".
// Pass "" to auto-detect via migrationsDir.
func MigrateStatus(ctx context.Context, cfg *config.Config, dir string) ([]MigrationStatus, error) {
	if dir == "" {
		dir = migrationsDir(cfg, "")
	}
	files, err := scanMigrations(dir)
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
	if need, needErr := ledgerNeedsUpgrade(ctx, cfg, files); needErr != nil {
		return nil, fmt.Errorf("check migration ledger: %w", needErr)
	} else if need {
		_, _ = fmt.Fprintln(statusWarnOut, ledgerUpgradeWarning)
	}

	var statuses []MigrationStatus
	onDisk := make(map[string]bool)

	for _, f := range files {
		name := migrationKey(f)
		onDisk[name] = true
		ts, ok := applied[name]
		statuses = append(statuses, MigrationStatus{
			Name:      name,
			Applied:   ok,
			Timestamp: ts,
		})
	}

	// Include any applied migrations not found on disk (orphans).
	for name, ts := range applied {
		if !onDisk[name] {
			statuses = append(statuses, MigrationStatus{
				Name:      name,
				Applied:   true,
				Timestamp: ts,
			})
		}
	}

	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Name < statuses[j].Name
	})
	return statuses, nil
}
