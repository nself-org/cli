package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// Purpose: the --migration-dir companions that make a directory migration
// previewable and reversible: PendingDirMigrations (read-only preview behind
// `db migrate up --migration-dir --dry-run`) and MigrateDownDir (`db migrate
// down --migration-dir [--steps N]`).
// Inputs: a *config.Config, a migrations directory, and (down) a step count.
// Outputs: the ledger names that are pending / were reverted, or an error.
// Constraints (P7-PROD-77): the preview issues SELECT statements only, never
// DDL, never the ledger-upgrade writes MigrateUp performs, and never a
// BEGIN...ROLLBACK probe. Down reverts exactly one directory migration per
// step, each in one transaction with both ledger deletes; every step is
// resolved and its down file read before the first one runs, so a missing
// file can never leave a half-applied rollback.

// ledgerObjectExists reports whether a schema-qualified table exists, using a
// read-only catalog lookup (to_regclass returns NULL, never errors, when the
// schema or table is missing).
func ledgerObjectExists(ctx context.Context, cfg *config.Config, qualified string) (bool, error) {
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}
	out, err := querySQL(ctx, cfg, db, fmt.Sprintf(
		"SELECT CASE WHEN to_regclass('%s') IS NULL THEN 'no' ELSE 'yes' END", qualified))
	if err != nil {
		return false, fmt.Errorf("check %s: %w", qualified, err)
	}
	return strings.TrimSpace(out) == "yes", nil
}

// opsChecksums reads name -> stored checksum from nself_ops.migrations
// (read-only); an absent table yields an empty map.
func opsChecksums(ctx context.Context, cfg *config.Config) (map[string]string, error) {
	sums := make(map[string]string)
	exists, err := ledgerObjectExists(ctx, cfg, "nself_ops.migrations")
	if err != nil || !exists {
		return sums, err
	}
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}
	out, err := querySQL(ctx, cfg, db, "SELECT name || '|' || checksum FROM nself_ops.migrations")
	if err != nil {
		return nil, fmt.Errorf("read migration checksums: %w", err)
	}
	for _, line := range strings.Split(out, "\n") {
		if parts := strings.SplitN(strings.TrimSpace(line), "|", 2); len(parts) == 2 {
			sums[parts[0]] = parts[1]
		}
	}
	return sums, nil
}

// PendingDirMigrations lists the files in dir that MigrateUpDir would apply,
// without writing anything. It applies the same decisions as MigrateUpDir and
// ApplyFile: ledger name = file base name, skip when recorded in
// np_common.schema_versions, fail on a checksum mismatch against
// nself_ops.migrations, reject SQL ValidateMigrationSQL refuses, and refuse on
// the ALTER prerequisite check. Missing ledger tables mean "nothing applied
// yet" (the real run would create them); nothing is created here.
func PendingDirMigrations(ctx context.Context, cfg *config.Config, dir string) ([]string, error) {
	files, err := scanMigrations(dir)
	if err != nil {
		return nil, err
	}
	applied, err := readOnlyApplied(ctx, cfg)
	if err != nil {
		return nil, err
	}
	sums, err := opsChecksums(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var pendingFiles, names []string
	for _, f := range files {
		name := filepath.Base(f)
		if err := validateMigrationName(name); err != nil {
			return nil, err
		}
		data, readErr := os.ReadFile(f)
		if readErr != nil {
			return nil, fmt.Errorf("read migration file %s: %w", name, readErr)
		}
		sum, csErr := checksumBytes(data)
		if csErr != nil {
			return nil, fmt.Errorf("compute checksum for %s: %w", name, csErr)
		}
		if _, ok := applied[name]; ok {
			if stored := strings.TrimSpace(sums[name]); stored != "" && stored != sum {
				return nil, fmt.Errorf("migration %s: checksum mismatch (stored %s, file %s) — file was modified after apply; manual intervention required", name, stored, sum)
			}
			continue
		}
		if lintErr := ValidateMigrationSQL(name, string(data)); lintErr != nil {
			return nil, lintErr
		}
		pendingFiles = append(pendingFiles, f)
		names = append(names, migrationKey(f))
	}
	if missing, prereqErr := checkAlterPrerequisites(ctx, cfg, pendingFiles); prereqErr != nil {
		return nil, fmt.Errorf("check migration prerequisites: %w", prereqErr)
	} else if len(missing) > 0 {
		return nil, prerequisiteError(missing)
	}
	return names, nil
}

// readOnlyApplied returns the applied set, or an empty one when the legacy
// ledger table does not exist yet. It never creates the table.
func readOnlyApplied(ctx context.Context, cfg *config.Config) (map[string]struct{}, error) {
	exists, err := ledgerObjectExists(ctx, cfg, "np_common.schema_versions")
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{})
	if !exists {
		return set, nil
	}
	applied, err := appliedMigrations(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("check applied migrations: %w", err)
	}
	for name := range applied {
		set[name] = struct{}{}
	}
	return set, nil
}

// downTxControl matches a transaction-control statement at the start of a
// line. A down file holding one would end the wrapping transaction early and
// run the ledger deletes outside it, so such a file is refused. `END;` is not
// listed: it legitimately closes plpgsql blocks.
var downTxControl = regexp.MustCompile(`(?im)^\s*(begin|start\s+transaction|commit|rollback|abort)(\s+(work|transaction))?\s*;`)

// downStep is one resolved rollback: the ledger name and its down SQL.
type downStep struct {
	name string
	sql  string
}

// resolveDownFile finds <name>_down.sql or <name>.down.sql in dir for the
// flat migration "<name>.sql". A missing file is an error naming both paths.
func resolveDownFile(dir, ledgerName string) (string, error) {
	stem := strings.TrimSuffix(ledgerName, ".sql")
	candidates := []string{
		filepath.Join(dir, stem+"_down.sql"),
		filepath.Join(dir, stem+".down.sql"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("down migration not found for %s: expected %s or %s",
		ledgerName, candidates[0], candidates[1])
}

// MigrateDownDir reverts the `steps` most recently applied migrations, which
// must all be files of dir. If the newest applied ledger entry is not a file
// in dir, or its down file is missing, nothing is reverted and the error names
// it: a rollback never reaches outside the named directory. Each step runs
// its down SQL and both ledger deletes in one transaction, so a failing step
// leaves its own ledger rows in place. Returns the names reverted.
func MigrateDownDir(ctx context.Context, cfg *config.Config, dir string, steps int) ([]string, error) {
	if steps < 1 {
		return nil, fmt.Errorf("--steps must be at least 1, got %d", steps)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("migrations directory not found: %s", dir)
	}
	if err := ensureSchemaVersions(ctx, cfg); err != nil {
		return nil, fmt.Errorf("ensure schema_versions: %w", err)
	}
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}
	out, err := querySQL(ctx, cfg, db, fmt.Sprintf(
		"SELECT name FROM np_common.schema_versions ORDER BY applied_at DESC, name DESC LIMIT %d", steps))
	if err != nil {
		return nil, fmt.Errorf("query latest migrations: %w", err)
	}
	var plan []downStep
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		step, planErr := planDownStep(dir, name)
		if planErr != nil {
			return nil, planErr
		}
		plan = append(plan, step)
	}
	if len(plan) == 0 {
		return nil, fmt.Errorf("no migrations to revert")
	}
	if len(plan) < steps {
		return nil, fmt.Errorf("only %d applied migration(s) in the ledger, --steps %d requested; nothing reverted", len(plan), steps)
	}
	var reverted []string
	for _, s := range plan {
		if err := pipeSQLToContainer(ctx, cfg, downTxSQL(s)); err != nil {
			return reverted, fmt.Errorf("revert %s: %w: %v", s.name, errs.ErrMigrationFailed, err)
		}
		reverted = append(reverted, s.name)
	}
	return reverted, nil
}

// planDownStep validates one ledger name against dir and reads its down SQL.
func planDownStep(dir, name string) (downStep, error) {
	if err := validateMigrationName(name); err != nil {
		return downStep{}, fmt.Errorf("migration name from schema_versions: %w", err)
	}
	if !strings.HasSuffix(name, ".sql") {
		return downStep{}, fmt.Errorf("latest applied migration %s is not a flat .sql file of %s; refusing to revert it", name, dir)
	}
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		return downStep{}, fmt.Errorf("latest applied migration %s is not in %s; refusing to revert a migration outside the directory", name, dir)
	}
	downPath, err := resolveDownFile(dir, name)
	if err != nil {
		return downStep{}, err
	}
	data, err := os.ReadFile(downPath)
	if err != nil {
		return downStep{}, fmt.Errorf("read down migration %s: %w", downPath, err)
	}
	if downTxControl.MatchString(stripSQLComments(string(data))) {
		return downStep{}, fmt.Errorf("down migration %s contains its own transaction control (BEGIN/COMMIT/ROLLBACK); remove it, the CLI wraps each step in one transaction", downPath)
	}
	return downStep{name: name, sql: string(data)}, nil
}

// downTxSQL wraps the down SQL and both ledger deletes in one transaction
// (same shape as MigrateDown, plus the lock/statement timeouts MigrateUpDir
// uses so a stuck rollback aborts instead of blocking production).
func downTxSQL(s downStep) string {
	quoted := strings.ReplaceAll(s.name, "'", "''")
	return "BEGIN;\n" +
		"SET LOCAL lock_timeout = '5s';\n" +
		"SET LOCAL statement_timeout = '60s';\n" +
		s.sql + "\n" +
		fmt.Sprintf("DELETE FROM np_common.schema_versions WHERE name = '%s';\n", quoted) +
		fmt.Sprintf("DELETE FROM nself_ops.migrations WHERE name = '%s';\n", quoted) +
		"COMMIT;\n"
}
