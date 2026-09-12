package database

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
)

// RestoreDrillResult describes the outcome of a restore test.
type RestoreDrillResult struct {
	StartedAt      time.Time `json:"started_at"`
	CompletedAt    time.Time `json:"completed_at"`
	BackupFile     string    `json:"backup_file"`
	Success        bool      `json:"success"`
	TablesVerified int       `json:"tables_verified"`
	RowsVerified   int64     `json:"rows_verified"`
	// MissingCriticalTables lists the entries of CriticalTables that were not
	// found (by name, any schema) in the restored database. Informational —
	// see the naming-mismatch note in drill.go: several drilled environments
	// do not use the np_ prefix, so this is reported rather than failing the
	// drill outright.
	MissingCriticalTables []string      `json:"missing_critical_tables,omitempty"`
	ErrorMessage          string        `json:"error_message,omitempty"`
	Duration              time.Duration `json:"duration_ns"`
}

// RestoreDrill runs a restore drill using the most recent backup.
// It restores to a TEMPORARY database named {dbname}_drilltest, verifies
// data integrity, then drops the temporary database.
// This is non-destructive — it does not touch the production database.
func RestoreDrill(ctx context.Context, cfg *config.Config, backupFile string) (RestoreDrillResult, error) {
	result := RestoreDrillResult{
		StartedAt: time.Now(),
	}

	// Find most recent backup if not specified.
	if backupFile == "" {
		found, err := mostRecentBackup(cfg)
		if err != nil {
			return result, fmt.Errorf("find most recent backup: %w", err)
		}
		backupFile = found
	}
	result.BackupFile = backupFile

	// Derive drill database name.
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}
	drillDB := db + "_drilltest"

	// Validate and double-quote the drill DB identifier before any DDL. SEC-SQL-01.
	quotedDrillDB, err := SanitizeIdentifier(drillDB)
	if err != nil {
		return result, fmt.Errorf("invalid drill database name %q: %w", drillDB, err)
	}

	// Ensure drill database does not already exist (clean state).
	_ = runSQLOnDB(ctx, cfg, "postgres", "DROP DATABASE IF EXISTS "+quotedDrillDB)

	// Create the temporary drill database.
	if err := runSQLOnDB(ctx, cfg, "postgres", "CREATE DATABASE "+quotedDrillDB); err != nil {
		return result, fmt.Errorf("create drill database %s: %w", drillDB, err)
	}

	// Restore backup into the drill database.
	// We build a restore-like command targeting drillDB directly.
	if err := restoreToDB(ctx, cfg, backupFile, drillDB); err != nil {
		_ = runSQLOnDB(ctx, cfg, "postgres", "DROP DATABASE IF EXISTS "+quotedDrillDB)
		result.ErrorMessage = err.Error()
		result.CompletedAt = time.Now()
		result.Duration = result.CompletedAt.Sub(result.StartedAt)
		_ = RecordDrillResult(".", result)
		return result, fmt.Errorf("restore into drill database: %w", err)
	}

	// Verify the restored database.
	tables, rows, err := VerifyRestoredDatabase(ctx, cfg, drillDB)
	if err != nil {
		_ = runSQLOnDB(ctx, cfg, "postgres", "DROP DATABASE IF EXISTS "+quotedDrillDB)
		result.ErrorMessage = err.Error()
		result.CompletedAt = time.Now()
		result.Duration = result.CompletedAt.Sub(result.StartedAt)
		_ = RecordDrillResult(".", result)
		return result, fmt.Errorf("verify drill database: %w", err)
	}

	result.TablesVerified = tables
	result.RowsVerified = rows

	// Which of the canonical CriticalTables actually exist by name. Query
	// while the scratch DB is still alive — the caller in drill.go cannot
	// requery after this function drops it below.
	missing, missErr := verifyCriticalTables(ctx, cfg, drillDB)
	if missErr != nil {
		_ = runSQLOnDB(ctx, cfg, "postgres", "DROP DATABASE IF EXISTS "+quotedDrillDB)
		result.ErrorMessage = missErr.Error()
		result.CompletedAt = time.Now()
		result.Duration = result.CompletedAt.Sub(result.StartedAt)
		_ = RecordDrillResult(".", result)
		return result, fmt.Errorf("verify critical tables: %w", missErr)
	}
	result.MissingCriticalTables = missing

	// Smoke gate: RestoreDrillResult.Success must be impossible to set true
	// without a positive assertion behind it — this cannot be left to whoever
	// calls RestoreDrill(). Before this check lived only in drill.go's
	// smokeCheck, reached exclusively through the Drill() wrapper; anything
	// that calls RestoreDrill() directly (cmd/commands/db_pitr_ops.go's
	// `nself db restore-drill`, and any future caller) got a RestoreDrillResult
	// with Success unconditionally true regardless of TablesVerified/
	// RowsVerified. That is the same hollow-gate shape proven live on staging
	// 2026-08-31 (tables_checked=107, rows_observed=0, success=true), just one
	// call-frame lower — reusing smokeCheck here so the invariant lives at the
	// one place that actually produces the result, not at every caller.
	if smokeErr := smokeCheck(result, ResolveCriticalTables(cfg)); smokeErr != nil {
		_ = runSQLOnDB(ctx, cfg, "postgres", "DROP DATABASE IF EXISTS "+quotedDrillDB)
		result.ErrorMessage = smokeErr.Error()
		result.CompletedAt = time.Now()
		result.Duration = result.CompletedAt.Sub(result.StartedAt)
		_ = RecordDrillResult(".", result)
		return result, fmt.Errorf("restore drill smoke check: %w", smokeErr)
	}

	// Drop the drill database.
	if err := runSQLOnDB(ctx, cfg, "postgres", "DROP DATABASE IF EXISTS "+quotedDrillDB); err != nil {
		return result, fmt.Errorf("drop drill database %s: %w", drillDB, err)
	}

	result.Success = true
	result.CompletedAt = time.Now()
	result.Duration = result.CompletedAt.Sub(result.StartedAt)

	if err := RecordDrillResult(".", result); err != nil {
		return result, fmt.Errorf("record drill result: %w", err)
	}

	return result, nil
}

// RecordDrillResult appends the drill result to .nself/restore-drills.log
// in JSON format (one JSON object per line).
func RecordDrillResult(projectDir string, result RestoreDrillResult) error {
	logPath := filepath.Join(projectDir, ".nself", "restore-drills.log")

	if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
		return fmt.Errorf("create .nself directory: %w", err)
	}

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal drill result: %w", err)
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open drill log %s: %w", logPath, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write drill log entry: %w", err)
	}

	return nil
}

// mostRecentBackup returns the path to the most recently modified .dump file
// in the configured backup directory.
func mostRecentBackup(cfg *config.Config) (string, error) {
	backupDir := cfg.Backup.Dir
	if backupDir == "" {
		backupDir = "backups"
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return "", fmt.Errorf("read backup directory %s: %w", backupDir, err)
	}

	type fileInfo struct {
		path    string
		modTime time.Time
	}
	var dumps []fileInfo

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".dump") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		dumps = append(dumps, fileInfo{
			path:    filepath.Join(backupDir, e.Name()),
			modTime: info.ModTime(),
		})
	}

	if len(dumps) == 0 {
		return "", fmt.Errorf("no .dump files found in %s", backupDir)
	}

	sort.Slice(dumps, func(i, j int) bool {
		return dumps[i].modTime.After(dumps[j].modTime)
	})

	return dumps[0].path, nil
}
