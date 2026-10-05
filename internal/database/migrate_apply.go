package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// Purpose: the query/apply surface of the migration runner: pending list,
// ApplyFile/MigrateUpDir/ApplyDir for external directories (G-008), merged
// on-disk/ledger status. Inputs: a *config.Config plus a plugin name, file or
// directory. Outputs: counts, skip flags or []MigrationStatus.
// Constraints: split out of migrate.go (CLI-R12); MigrateUp/MigrateDown stay there.

// ApplyFile applies a single external SQL migration file and records it in
// schema_versions by filename + SHA-256 checksum (G-008).
//
// Double-apply protection: if a file with the same filename is already present
// in schema_versions, the function logs the skip and returns (true, nil).
// The checksum is stored in nself_ops.migrations for audit purposes.
//
// This enables plugin-claw external RLS migrations to be applied via CLI
// without requiring 'nself db shell' as a workaround.
func ApplyFile(ctx context.Context, cfg *config.Config, filePath string) (skipped bool, err error) {
	return applyFile(ctx, cfg, filePath, false, nil)
}

// applyFile is ApplyFile; guarded is set by MigrateUpDir (P7-PROD-77): the
// transactional path then runs the file through dirSQL and dirTxSQL (outer
// wrapper dropped, transaction control refused, same-transaction check, ledger
// first) and verifies the ledger row. `db migrate apply --file` stays unguarded
// and is the escape hatch for a file the guard refuses.
//
// led is the ledger MigrateUpDir read once for the whole run (P7-LIVE-11): with
// it the file is neither preceded by the table ensure nor by a ledger read, and
// a file that applies is added to led. With nil (ApplyFile) both happen here.
func applyFile(ctx context.Context, cfg *config.Config, filePath string, guarded bool, led *ledgerSnapshot) (skipped bool, err error) {
	if led == nil {
		if err := ensureSchemaVersions(ctx, cfg); err != nil {
			return false, fmt.Errorf("ensure schema_versions: %w", err)
		}
		if err := ensureMigrationsTable(ctx, cfg); err != nil {
			return false, fmt.Errorf("ensure migrations table: %w", err)
		}
	}

	name := filepath.Base(filePath)
	if err := validateMigrationName(name); err != nil {
		return false, err
	}

	data, readErr := os.ReadFile(filePath)
	if readErr != nil {
		return false, fmt.Errorf("read migration file %s: %w", name, readErr)
	}

	// Reject SQL no Postgres version accepts before opening a transaction, so
	// it surfaces here rather than as a syntax error partway through a batch
	// with earlier statements already committed.
	if lintErr := ValidateMigrationSQL(name, string(data)); lintErr != nil {
		return false, lintErr
	}

	checksum, csErr := checksumBytes(data)
	if csErr != nil {
		return false, fmt.Errorf("compute checksum for %s: %w", name, csErr)
	}
	if !sha256HexRegex.MatchString(checksum) {
		return false, fmt.Errorf("unexpected checksum format for %s", name)
	}

	// Check if this filename is already in schema_versions.
	if led == nil {
		applied, err := appliedMigrations(ctx, cfg)
		if err != nil {
			return false, fmt.Errorf("check applied migrations: %w", err)
		}
		led = &ledgerSnapshot{applied: applied}
		if _, ok := applied[name]; ok {
			db := cfg.Postgres.DB
			if db == "" {
				db = "nself"
			}
			if stored, queryErr := querySQL(ctx, cfg, db, "SELECT checksum FROM nself_ops.migrations WHERE name = '"+strings.ReplaceAll(name, "'", "''")+"'"); queryErr == nil {
				led.sums = map[string]string{name: stored}
			}
		}
	} else {
		defer func() {
			if err == nil && !skipped {
				led.applied[name] = time.Now()
				led.sums[name] = checksum
			}
		}()
	}
	if _, ok := led.applied[name]; ok {
		// Already applied: verify the checksum to detect file modifications.
		if stored := strings.TrimSpace(led.sums[name]); stored != "" && stored != checksum {
			return false, fmt.Errorf("migration %s: checksum mismatch (stored %s, file %s) — file was modified after apply; manual intervention required", name, stored, checksum)
		}
		// Already applied with matching checksum — skip without error.
		return true, nil
	}

	migrationID := extractMigrationID(filePath)
	if migrationID == "" {
		// Fall back to the full filename as ID when nothing could be derived.
		migrationID = name
	}
	if err := validateMigrationName(migrationID); err != nil {
		return false, fmt.Errorf("migration id from %s: %w", name, err)
	}

	legacyRecord, opsRecord := migrationRecordSQL(migrationID, name, checksum)

	sqlContent := string(data)

	if isNonTransactional(sqlContent) {
		if err := pipeSQLToContainer(ctx, cfg, sqlContent); err != nil {
			return false, fmt.Errorf("migration %s: %w: %v", name, errs.ErrMigrationFailed, err)
		}
		// Record both tables atomically in a separate transaction so that
		// a failure on either INSERT leaves no orphan row in the other table.
		recordSQL := "BEGIN;\n" + legacyRecord + "\n" + opsRecord + "\nCOMMIT;\n"
		if err := pipeSQLToContainer(ctx, cfg, recordSQL); err != nil {
			return false, fmt.Errorf("record migration %s: %w", name, err)
		}
	} else if guarded {
		body, txErr := dirSQL(name, sqlContent, "up")
		if txErr != nil {
			return false, txErr
		}
		if err := pipeSQLToContainer(ctx, cfg, dirTxSQL(body, legacyRecord+"\n"+opsRecord)); err != nil {
			return false, txRunError(name, err)
		}
		if err := verifyLedger(ctx, cfg, name, true); err != nil {
			return false, err
		}
	} else {
		txSQL := "BEGIN;\n" +
			"SET LOCAL lock_timeout = '5s';\n" +
			"SET LOCAL statement_timeout = '60s';\n" +
			sqlContent + "\n" + legacyRecord + "\n" + opsRecord + "\nCOMMIT;\n"
		if err := pipeSQLToContainer(ctx, cfg, txSQL); err != nil {
			return false, fmt.Errorf("migration %s: %w: %v", name, errs.ErrMigrationFailed, err)
		}
	}
	return false, nil
}

// MigrateUpDir applies all .sql files in the given directory in lexicographic
// order, skipping any already recorded in schema_versions (idempotent).
// This is the --migration-dir companion to ApplyFile (G-008).
func MigrateUpDir(ctx context.Context, cfg *config.Config, dir string) (int, error) {
	return MigrateUpDirProgress(ctx, cfg, dir, nil)
}

// MigrateUpDirProgress is MigrateUpDir with a progress callback, called as
// progress(n, total, key) just before the n-th of total pending files runs
// (nil: silent). The ledger tables are ensured once and the ledger is read once
// per run (P7-LIVE-11, EPIC D19); each pending file still runs in its own
// transaction, so a no-op rerun costs the same few execs for 1 or 58 files.
func MigrateUpDirProgress(ctx context.Context, cfg *config.Config, dir string, progress func(n, total int, file string)) (int, error) {
	if err := ensureLedgerTables(ctx, cfg); err != nil {
		return 0, fmt.Errorf("ensure ledger tables: %w", err)
	}

	files, err := scanMigrations(dir)
	if err != nil {
		return 0, err
	}

	// Same ALTER-prerequisite refusal MigrateUp applies (migrate_prereq.go),
	// scoped to this directory's still-pending files.
	led, err := readLedger(ctx, cfg)
	if err != nil {
		return 0, err
	}
	pending := pendingMigrationFiles(files, led.applied)
	if missing, prereqErr := checkAlterPrerequisites(ctx, cfg, pending); prereqErr != nil {
		return 0, fmt.Errorf("check migration prerequisites: %w", prereqErr)
	} else if len(missing) > 0 {
		return 0, prerequisiteError(missing)
	}

	// Refuse the whole batch before applying any file (non-transactional
	// files run as written, as before, and are not scanned).
	for _, f := range pending {
		if data, readErr := os.ReadFile(f); readErr == nil && !isNonTransactional(string(data)) {
			if _, txErr := dirSQL(filepath.Base(f), string(data), "up"); txErr != nil {
				return 0, txErr
			}
		}
	}

	pendingSet := make(map[string]bool, len(pending))
	for _, f := range pending {
		pendingSet[f] = true
	}
	count, n := 0, 0
	for _, f := range files {
		if progress != nil && pendingSet[f] {
			n++
			progress(n, len(pending), migrationKey(f))
		}
		skipped, applyErr := applyFile(ctx, cfg, f, true, led)
		if applyErr != nil {
			return count, applyErr
		}
		if !skipped {
			count++
		}
	}
	return count, nil
}

// ApplyDir is an alias for MigrateUpDir with the name the task spec expects (G-008).
func ApplyDir(ctx context.Context, cfg *config.Config, dirPath string) (int, error) {
	return MigrateUpDir(ctx, cfg, dirPath)
}

// MigrateStatus returns the status of all known migrations (applied and pending).
// It merges on-disk migration files with the schema_versions table, so orphaned
// migrations (applied but no longer on disk) are also reported.
//
// dir overrides the auto-detected migrations directory (the --migration-dir
// companion to MigrateUpDir, G-008): repos with non-standard layouts, e.g.
// ntask's postgres/migrations, would otherwise report "No migrations found".
// Pass "" to auto-detect via migrationsDir.
func MigrateStatus(ctx context.Context, cfg *config.Config, dir string) ([]MigrationStatus, error) {
	if err := ensureSchemaVersions(ctx, cfg); err != nil {
		return nil, fmt.Errorf("ensure schema_versions: %w", err)
	}
	if err := ensureMigrationsTable(ctx, cfg); err != nil {
		return nil, fmt.Errorf("ensure migrations table: %w", err)
	}

	if dir == "" {
		dir = migrationsDir(cfg, "")
	}
	files, err := scanMigrations(dir)
	if err != nil {
		return nil, err
	}

	if err := upgradeLedger(ctx, cfg, files); err != nil {
		return nil, fmt.Errorf("upgrade migration ledger: %w", err)
	}

	applied, err := appliedMigrations(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("check applied migrations: %w", err)
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
