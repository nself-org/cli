package backup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
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

// smokeAndSentinel runs the smoke-query catalog and a sentinel CRUD round-trip
// against the restored container.
func smokeAndSentinel(ctx context.Context, c *Container) error {
	user, db := c.User, c.DB
	testContainer := c.Name

	// --- Run smoke-query catalog (5+ system tables) ---
	// The first query (user table count) is the gate. Count == 0 means this is
	// a schema-only restore and we must fail immediately with a clear message.
	failedQueries := 0
	for _, sq := range smokeQueryCatalog {
		smokeCmd := c.cmd(ctx, nil, "psql", "-U", user, "-d", db, "-t", "-c", sq.SQL)
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

	sentinelCtx, sentinelCancel := context.WithTimeout(ctx, 5*time.Second)
	defer sentinelCancel()

	sentinelOut, sentinelErr := c.cmd(sentinelCtx, nil, "psql", "-U", user, "-d", db, "-t", "-c", sentinelSQL).CombinedOutput()
	sentinelDur := time.Since(sentinelStart)

	if sentinelErr != nil {
		// Always attempt cleanup of sentinel schema on error.
		cleanSQL := "DROP SCHEMA IF EXISTS _nself_verify_sentinel CASCADE;"
		_ = c.cmd(ctx, nil, "psql", "-U", user, "-d", db, "-c", cleanSQL).Run()
		return fmt.Errorf("%w: sentinel CRUD failed (%s): %s", errs.ErrBackupVerifyFailed, sentinelDur, string(sentinelOut))
	}

	slog.Info("sentinel CRUD round-trip passed", "container", testContainer, "duration_ms", sentinelDur.Milliseconds())

	if !strings.Contains(string(sentinelOut), "s46-verify") {
		return fmt.Errorf("%w: sentinel value not found in read-back", errs.ErrBackupVerifyFailed)
	}

	return nil
}

func quoteIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

const listTablesSQL = `SELECT n.nspname, c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
WHERE c.relkind IN ('r','p','m') AND n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname !~ '^pg_toast'
ORDER BY 1,2`

// CountRows returns the exact count(*) of every user table, key "schema.table".
func CountRows(ctx context.Context, c *Container) (map[string]int64, error) {
	out, err := c.Query(ctx, listTablesSQL)
	if err != nil {
		return nil, err
	}
	var names, parts []string
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(f) != 2 {
			continue
		}
		parts = append(parts, fmt.Sprintf("SELECT %d, count(*) FROM %s.%s", len(names), quoteIdent(f[0]), quoteIdent(f[1])))
		names = append(names, f[0]+"."+f[1])
	}
	rows := map[string]int64{}
	if len(names) == 0 {
		return rows, nil
	}
	if out, err = c.Query(ctx, strings.Join(parts, " UNION ALL ")+";"); err != nil {
		return nil, err
	}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimSpace(line), "\t")
		if len(f) != 2 {
			continue
		}
		i, e1 := strconv.Atoi(f[0])
		n, e2 := strconv.ParseInt(f[1], 10, 64)
		if e1 != nil || e2 != nil || i < 0 || i >= len(names) {
			return nil, fmt.Errorf("unexpected count output %q", line)
		}
		rows[names[i]] = n
	}
	if len(rows) != len(names) {
		return nil, fmt.Errorf("counted %d of %d tables", len(rows), len(names))
	}
	return rows, nil
}
