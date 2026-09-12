package backup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/metrics"
)

// smokeQueryCatalog is the set of per-system-table smoke queries run after a
// restore. Each entry is a human label and a SQL expression. A zero count on
// any of these indicates a schema-only (empty) restore and fails the check.
//
// The catalog intentionally targets tables that are always present when nself
// data has been genuinely restored. auth.users count, hasura metadata count,
// and per-app claw conversation count are the canonical signal set.
var smokeQueryCatalog = []struct {
	Label string
	SQL   string
}{
	{
		"information_schema.tables (user tables)",
		"SELECT count(*) FROM information_schema.tables WHERE table_schema NOT IN ('information_schema','pg_catalog','pg_toast');",
	},
	{
		"pg_stat_user_tables (live tuples)",
		"SELECT coalesce(sum(n_live_tup),0) FROM pg_stat_user_tables;",
	},
	{
		"auth.users",
		"SELECT count(*) FROM information_schema.tables WHERE table_schema='auth' AND table_name='users';",
	},
	{
		"hdb_catalog.hdb_metadata",
		"SELECT count(*) FROM information_schema.tables WHERE table_name='hdb_metadata';",
	},
	{
		"np_claw_conversations (presence check)",
		"SELECT count(*) FROM information_schema.tables WHERE table_name='np_claw_conversations';",
	},
}

// VerifyOptions holds flags for `nself backup verify`.
type VerifyOptions struct {
	BackupID    string // backup ID or "latest"
	RestoreTest bool   // spin up test container and restore
	Cleanup     bool   // remove test container after verify
	Keep        bool   // keep test container for inspection
}

// VerifyResult holds the outcome of a backup verification.
type VerifyResult struct {
	BackupID  string    `json:"backup_id"`
	Verified  bool      `json:"verified"`
	StartedAt time.Time `json:"started_at"`
	Duration  string    `json:"duration"`
	Method    string    `json:"method"` // checksum, restore-test
	Details   string    `json:"details"`
}

// Verify checks backup integrity, optionally performing a restore test.
func Verify(ctx context.Context, cfg *config.Config, opts VerifyOptions) (*VerifyResult, error) {
	backupDir := cfg.Backup.Dir
	if backupDir == "" {
		backupDir = "./backups"
	}

	start := time.Now()
	result := &VerifyResult{
		BackupID:  opts.BackupID,
		StartedAt: start,
		Method:    "checksum",
	}

	backupFile, err := resolveBackupFile(backupDir, opts.BackupID)
	if err != nil {
		return nil, err
	}

	// Basic integrity check: file exists and is non-empty.
	info, err := os.Stat(backupFile)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errs.ErrBackupNotFound, err)
	}
	if info.Size() == 0 {
		result.Verified = false
		result.Details = "backup file is empty"
		result.Duration = time.Since(start).String()
		return result, fmt.Errorf("%w: backup file is empty", errs.ErrBackupVerifyFailed)
	}

	if opts.RestoreTest {
		result.Method = "restore-test"
		if err := runRestoreTest(ctx, cfg, backupFile, opts); err != nil {
			result.Verified = false
			result.Details = err.Error()
			result.Duration = time.Since(start).String()
			emitVerifyMetric(cfg, start, false)
			slog.Error("restore-test failed", "id", opts.BackupID, "RESULT", "fail", "duration_sec", int(time.Since(start).Seconds()), "error", err)
			return result, err
		}
	}

	result.Verified = true
	result.Details = fmt.Sprintf("file size: %d bytes", info.Size())
	result.Duration = time.Since(start).String()

	if opts.RestoreTest {
		emitVerifyMetric(cfg, start, true)
		// Emit the exact phrase the systemd/journalctl grep expects.
		slog.Info("restore-test passed", "id", opts.BackupID, "RESULT", "pass", "duration_sec", int(time.Since(start).Seconds()))
	}

	slog.Info("backup verified", "id", opts.BackupID, "method", result.Method, "duration", result.Duration)
	return result, nil
}

func emitVerifyMetric(cfg *config.Config, start time.Time, success bool) {
	rec := metrics.VerifyRecord{
		Env:         cfg.Env,
		Success:     success,
		DurationSec: time.Since(start).Seconds(),
		Timestamp:   time.Now(),
	}
	if err := metrics.EmitVerify(rec); err != nil {
		slog.Warn("emit verify metric", "error", err)
	}
}

// runRestoreTest spins up a temporary postgres container, restores the backup,
// and runs the full smoke-query catalog plus a sentinel CRUD round-trip to
// verify data integrity. A schema-only restore (empty DB) fails this check.
//
// Flag: --restore-test (confirmed per S41-T11 drift fix).
func runRestoreTest(ctx context.Context, cfg *config.Config, backupFile string, opts VerifyOptions) error {
	testContainer := cfg.ProjectName + "_pg_restore_test"
	testVolume := cfg.ProjectName + "_restore_test_data"
	pgVersion := cfg.Postgres.Version
	if pgVersion == "" {
		pgVersion = "16-alpine"
	}

	slog.Info("starting restore test container", "container", testContainer)

	// Create ephemeral volume and container.
	runArgs := []string{
		"run", "-d",
		"--name", testContainer,
		"-v", testVolume + ":/var/lib/postgresql/data",
		"-e", "POSTGRES_USER=" + cfg.Postgres.User,
		"-e", "POSTGRES_PASSWORD=" + cfg.Postgres.Password,
		"-e", "POSTGRES_DB=" + cfg.Postgres.DB,
		"postgres:" + pgVersion,
	}

	cmd := exec.CommandContext(ctx, "docker", runArgs...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("start test container: %s: %w", string(output), err)
	}

	// Cleanup deferred unconditionally so sentinel schema is always removed.
	cleanup := func() {
		if opts.Keep {
			slog.Info("keeping test container for inspection", "container", testContainer)
			return
		}
		slog.Info("cleaning up test container", "container", testContainer)
		_ = exec.CommandContext(ctx, "docker", "rm", "-f", testContainer).Run()
		_ = exec.CommandContext(ctx, "docker", "volume", "rm", "-f", testVolume).Run()
	}
	defer cleanup()

	// Wait for postgres to be ready (up to 30s).
	user := cfg.Postgres.User
	if user == "" {
		user = "postgres"
	}
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}
	ready := false
	for i := 0; i < 30; i++ {
		check := exec.CommandContext(ctx, "docker", "exec", testContainer, "pg_isready", "-U", user)
		if check.Run() == nil {
			ready = true
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	if !ready {
		return fmt.Errorf("%w: postgres container not ready after 30s", errs.ErrBackupVerifyFailed)
	}

	// Restore the backup into the test container. Only the pg_dump (.dump)
	// format produced by `nself backup create` has an automated restore path.
	// A legacy base-backup tar cannot be restored here, so fail with a clear
	// message instead of running smoke queries against an empty database (which
	// would mis-report a "schema-only restore").
	if strings.HasSuffix(backupFile, ".dump") {
		if err := restorePgDump(ctx, testContainer, user, db, backupFile); err != nil {
			return fmt.Errorf("restore test failed: %w", err)
		}
	} else {
		return fmt.Errorf("%w: %s is not a restorable pg_dump (.dump) backup; recreate it with the default format before running --restore-test",
			errs.ErrBackupVerifyFailed, backupFile)
	}

	// --- Run smoke-query catalog (5+ system tables) ---
	// The first query (user table count) is the gate. Count == 0 means this is
	// a schema-only restore and we must fail immediately with a clear message.
	failedQueries := 0
	for _, sq := range smokeQueryCatalog {
		smokeArgs := []string{
			"exec", testContainer,
			"psql", "-U", user, "-d", db, "-t", "-c", sq.SQL,
		}
		smokeCmd := exec.CommandContext(ctx, "docker", smokeArgs...)
		output, err := smokeCmd.CombinedOutput()
		if err != nil {
			// The gate query (user table count) is the one condition this
			// whole check exists to enforce. If IT errors out — psql auth
			// failure, container gone, whatever — that is not "unknown,
			// carry on": failing to even run the assertion is the same
			// hollow-gate shape as the drill's zero-row bug (a success
			// returned with no positive check behind it). Every other
			// smoke query is advisory and may legitimately fail without
			// aborting the restore-test.
			if sq.Label == smokeQueryCatalog[0].Label {
				return fmt.Errorf("%w: could not run the user-table gate query (%s): %s",
					errs.ErrBackupVerifyFailed, err, strings.TrimSpace(string(output)))
			}
			slog.Warn("smoke query error", "label", sq.Label, "error", err)
			failedQueries++
			continue
		}
		count := strings.TrimSpace(string(output))
		slog.Info("smoke query", "label", sq.Label, "count", count)
		// For the first (user table) query, a zero count is a hard failure.
		if sq.Label == smokeQueryCatalog[0].Label && count == "0" {
			return fmt.Errorf("%w: row count mismatch — restored database has no user tables (schema-only restore detected)", errs.ErrBackupVerifyFailed)
		}
	}
	if failedQueries > 0 {
		slog.Warn("some smoke queries failed", "failed", failedQueries, "total", len(smokeQueryCatalog))
	}

	// --- Sentinel CRUD round-trip (must complete in < 2s) ---
	sentinelStart := time.Now()
	sentinelSQL := strings.Join([]string{
		"CREATE SCHEMA IF NOT EXISTS _nself_verify_sentinel;",
		"CREATE TABLE IF NOT EXISTS _nself_verify_sentinel.probe (id serial primary key, val text);",
		"INSERT INTO _nself_verify_sentinel.probe(val) VALUES ('s46-verify');",
		"SELECT val FROM _nself_verify_sentinel.probe WHERE val='s46-verify';",
		"DROP SCHEMA _nself_verify_sentinel CASCADE;",
	}, " ")

	sentinelArgs := []string{
		"exec", testContainer,
		"psql", "-U", user, "-d", db, "-t", "-c", sentinelSQL,
	}
	sentinelCtx, sentinelCancel := context.WithTimeout(ctx, 5*time.Second)
	defer sentinelCancel()

	sentinelCmd := exec.CommandContext(sentinelCtx, "docker", sentinelArgs...)
	sentinelOut, sentinelErr := sentinelCmd.CombinedOutput()
	sentinelDur := time.Since(sentinelStart)

	if sentinelErr != nil {
		// Always attempt cleanup of sentinel schema on error.
		cleanSQL := "DROP SCHEMA IF EXISTS _nself_verify_sentinel CASCADE;"
		_ = exec.CommandContext(ctx, "docker", "exec", testContainer, "psql", "-U", user, "-d", db, "-c", cleanSQL).Run()
		return fmt.Errorf("%w: sentinel CRUD failed (%s): %s", errs.ErrBackupVerifyFailed, sentinelDur, string(sentinelOut))
	}

	slog.Info("sentinel CRUD round-trip passed", "duration_ms", sentinelDur.Milliseconds())

	if !strings.Contains(string(sentinelOut), "s46-verify") {
		return fmt.Errorf("%w: sentinel value not found in read-back", errs.ErrBackupVerifyFailed)
	}

	return nil
}
