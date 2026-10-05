package database

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// Purpose: PendingDirMigrations (read-only `up --migration-dir --dry-run`),
// MigrateDownDir (`down --migration-dir [--steps N]`) and dirSQL, the shared
// transaction-control guard (P7-PROD-77). The preview issues SELECTs only; down
// runs one migration per step in one transaction with both ledger deletes.

// ledgerSelect runs one read-only query against the configured database.
func ledgerSelect(ctx context.Context, cfg *config.Config, q string) (string, error) {
	return querySQL(ctx, cfg, cmp.Or(cfg.Postgres.DB, "nself"), q)
}

// ledgerTableExists reports, read-only via to_regclass (NULL, never an error,
// for a missing schema or table), whether a schema-qualified table exists.
func ledgerTableExists(ctx context.Context, cfg *config.Config, qualified string) (bool, error) {
	out, err := ledgerSelect(ctx, cfg, fmt.Sprintf(
		"SELECT CASE WHEN to_regclass('%s') IS NULL THEN 'no' ELSE 'yes' END", qualified))
	if err != nil {
		return false, fmt.Errorf("check %s: %w", qualified, err)
	}
	return strings.TrimSpace(out) == "yes", nil
}

// opsChecksums reads name -> checksum from nself_ops.migrations (read-only).
func opsChecksums(ctx context.Context, cfg *config.Config) (map[string]string, error) {
	sums := make(map[string]string)
	exists, err := ledgerTableExists(ctx, cfg, "nself_ops.migrations")
	if err != nil || !exists {
		return sums, err
	}
	out, err := ledgerSelect(ctx, cfg, "SELECT name || '|' || checksum FROM nself_ops.migrations")
	if err != nil {
		return nil, fmt.Errorf("read migration checksums: %w", err)
	}
	for _, line := range strings.Split(out, "\n") {
		if n, c, ok := strings.Cut(strings.TrimSpace(line), "|"); ok {
			sums[n] = c
		}
	}
	return sums, nil
}

// PendingDirMigrations lists the files MigrateUpDir would apply, writing nothing,
// with the real run's refusals (checksum, lint, transaction control, ALTER
// prerequisites). Missing ledger tables mean "nothing applied"; none is created.
func PendingDirMigrations(ctx context.Context, cfg *config.Config, dir string) ([]string, error) {
	files, err := scanMigrations(dir)
	if err != nil {
		return nil, err
	}
	applied := map[string]time.Time{}
	if ok, err := ledgerTableExists(ctx, cfg, "np_common.schema_versions"); err != nil {
		return nil, err
	} else if ok {
		if applied, err = appliedMigrations(ctx, cfg); err != nil {
			return nil, fmt.Errorf("check applied migrations: %w", err)
		}
	}
	sums, err := opsChecksums(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var pendingFiles, names []string
	for _, f := range files {
		name := filepath.Base(f)
		data, readErr := os.ReadFile(f)
		if nameErr := validateMigrationName(name); nameErr != nil || readErr != nil {
			return nil, errors.Join(nameErr, readErr)
		}
		sum, _ := checksumBytes(data)
		if _, ok := applied[name]; ok {
			if stored := strings.TrimSpace(sums[name]); stored != "" && stored != sum {
				return nil, fmt.Errorf("migration %s: checksum mismatch (stored %s, file %s) — file was modified after apply; manual intervention required", name, stored, sum)
			}
			continue
		}
		if lintErr := ValidateMigrationSQL(name, string(data)); lintErr != nil {
			return nil, lintErr
		}
		if _, err := dirSQL(name, string(data), "up"); err != nil && !isNonTransactional(string(data)) {
			return nil, err
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

// Transaction guard for directory migrations (P7-PROD-77). Two layers, neither
// of which parses SQL:
//
//  1. dirSQL over-refuses: after normalising line endings and dropping only
//     the exact outer BEGIN;/COMMIT; wrapper, any COMMIT, ROLLBACK, ABORT,
//     SAVEPOINT, RELEASE or PREPARE as a word anywhere (strings and trailing
//     comments included; only whole lines starting with -- are skipped, as
//     they are inert wherever they sit) refuses the file. BEGIN, END, DO and CALL are not refused:
//     inside a function, procedure or DO block run in our transaction,
//     Postgres itself rejects transaction control, and CREATE INDEX
//     CONCURRENTLY errors inside a transaction block.
//  2. The server checks the one thing text cannot: dirTxSQL records
//     txid_current() in a transaction-local setting, writes the ledger change
//     FIRST, runs the file, then asserts the same transaction is still open
//     (a top-level END acts as COMMIT, and so would anything unforeseen)
//     before COMMIT. A mismatch fails with xactMismatchMarker.
var (
	txWordsRe  = regexp.MustCompile(`(?i)\b(commit|rollback|abort|savepoint|release|prepare)\b`)
	wrapHeadRe = regexp.MustCompile(`(?i)\A((?:\s|--[^\n]*\n)*)((?:begin(?:\s+transaction)?|start\s+transaction)\s*;)`)
	wrapTailRe = regexp.MustCompile(`(?i)\b(?:commit|end)\s*;(?:\s|--[^\n]*|/\*(?:[^*]|\*+[^*/])*\*+/)*\z`) // trailing comments (a commented DOWN section) go with it
	eolRe      = regexp.MustCompile("\r\n|\r|\u2028|\u2029")
	// A line that starts with -- is inert whatever surrounds it (comment, or
	// inside a string, block comment or dollar body), so it is not scanned.
	commentLineRe = regexp.MustCompile(`(?m)^[ \t]*--[^\n]*$`)
)

const (
	xactMismatchMarker = "NSELF_XACT_MISMATCH"
	xactMark           = "SELECT set_config('nself.migration_xact', txid_current()::text, true);\n"
	xactCheck          = "DO $nself$ BEGIN IF current_setting('nself.migration_xact', true) IS DISTINCT FROM txid_current()::text THEN RAISE EXCEPTION '" + xactMismatchMarker + "'; END IF; END $nself$;\n"
)

// dirSQL returns sql ready to run inside the CLI's transaction: line endings
// normalised, the exact outer wrapper (BEGIN|BEGIN TRANSACTION|START
// TRANSACTION first, COMMIT|END last) blanked. kind is "up" or "down" and only
// shapes the advice in the refusal.
func dirSQL(name, sql, kind string) (string, error) {
	out := eolRe.ReplaceAllString(sql, "\n")
	head := wrapHeadRe.FindStringSubmatchIndex(out)
	tail := wrapTailRe.FindStringIndex(out)
	if head != nil && tail != nil && tail[0] >= head[1] {
		mid := strings.TrimSpace(out[head[1]:tail[0]])
		lastLine := strings.TrimSpace(mid[strings.LastIndex(mid, "\n")+1:])
		if mid == "" || strings.HasSuffix(mid, ";") || strings.HasSuffix(mid, "*/") || strings.HasPrefix(lastLine, "--") {
			blank := func(lo, hi int) string { return strings.Repeat(" ", hi-lo) }
			out = out[:head[4]] + blank(head[4], head[1]) + out[head[1]:tail[0]] + blank(tail[0], len(out))
		}
	}
	scan := commentLineRe.ReplaceAllStringFunc(out, func(l string) string { return strings.Repeat(" ", len(l)) })
	if loc := txWordsRe.FindStringIndex(scan); loc != nil {
		advice := "run it by hand in 'nself db shell', then delete its two ledger rows (np_common.schema_versions, nself_ops.migrations)"
		if kind == "up" {
			advice = "run it with 'nself db migrate apply --file <path>' instead, which applies the file as written"
		}
		return "", fmt.Errorf("migration %s: %q at line %d is transaction control (only the outer BEGIN/COMMIT wrapper is accepted; comments and strings count too); %s",
			name, scan[loc[0]:loc[1]], 1+strings.Count(scan[:loc[0]], "\n"), advice)
	}
	return out, nil
}

// dirTxSQL wraps one migration in the CLI's transaction: timeouts, the
// transaction mark, the ledger change first, the file, the same-transaction
// check, COMMIT.
func dirTxSQL(body, ledger string) string {
	return "BEGIN;\nSET LOCAL lock_timeout = '5s';\nSET LOCAL statement_timeout = '60s';\n" +
		xactMark + ledger + "\n" + body + "\n" + xactCheck + "COMMIT;\n"
}

// txRunError turns a failed pipe into the migration error; a failed
// same-transaction check gets the loud reconcile message instead.
func txRunError(name string, err error) error {
	if strings.Contains(err.Error(), xactMismatchMarker) {
		return fmt.Errorf("migration %s: the file ended or restarted the transaction, so part of it may be COMMITTED; the ledger and schema may disagree. RECONCILE BY HAND before re-running: compare the schema with np_common.schema_versions and nself_ops.migrations ('nself db shell'): %w: %v", name, errs.ErrMigrationFailed, err)
	}
	return fmt.Errorf("migration %s: %w: %v", name, errs.ErrMigrationFailed, err)
}

// verifyLedger fails unless np_common.schema_versions has (present) or lacks
// the row after a run: a file that swallowed our COMMIT (an unterminated
// comment) would otherwise read as success.
func verifyLedger(ctx context.Context, cfg *config.Config, name string, present bool) error {
	out, err := ledgerSelect(ctx, cfg, fmt.Sprintf("SELECT count(*) FROM np_common.schema_versions WHERE name = '%s'", strings.ReplaceAll(name, "'", "''")))
	if err != nil || strings.TrimSpace(out) != map[bool]string{true: "1", false: "0"}[present] {
		return fmt.Errorf("migration %s: ledger row is not as expected after the run (present want %v): nothing was committed or the file swallowed the COMMIT; check np_common.schema_versions: %v", name, present, err)
	}
	return nil
}

type downStep struct{ name, sql string }

// MigrateDownDir reverts the `steps` newest applied migrations, all files of
// dir whose checksum equals the one recorded for that name (the ledger keys on
// the base name only). Every step is resolved before the first runs; each is
// one transaction with both ledger deletes. Returns the names reverted.
func MigrateDownDir(ctx context.Context, cfg *config.Config, dir string, steps int) ([]string, error) {
	if steps < 1 {
		return nil, fmt.Errorf("--steps must be at least 1, got %d", steps)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("migrations directory not found: %s", dir)
	}
	if err := ensureSchemaVersions(ctx, cfg); err != nil {
		return nil, err
	}
	out, err := ledgerSelect(ctx, cfg, fmt.Sprintf(
		"SELECT name FROM np_common.schema_versions ORDER BY applied_at DESC, name DESC LIMIT %d", steps))
	if err != nil {
		return nil, fmt.Errorf("query latest migrations: %w", err)
	}
	sums, err := opsChecksums(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var plan []downStep
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		step, planErr := planDownStep(dir, name, sums[name])
		if planErr != nil {
			return nil, planErr
		}
		plan = append(plan, step)
	}
	if len(plan) < steps { // also covers an empty ledger
		return nil, fmt.Errorf("only %d applied migration(s) in the ledger, --steps %d requested; nothing reverted", len(plan), steps)
	}
	var reverted []string
	for _, s := range plan {
		if err := pipeSQLToContainer(ctx, cfg, downTxSQL(s)); err != nil {
			return reverted, txRunError(s.name, err)
		}
		if err := verifyLedger(ctx, cfg, s.name, false); err != nil {
			return reverted, err
		}
		reverted = append(reverted, s.name)
	}
	return reverted, nil
}

// planDownStep checks a ledger name against dir (file present, checksum equal
// to the recorded one) and reads its down SQL.
func planDownStep(dir, name, storedSum string) (downStep, error) {
	if err := validateMigrationName(name); err != nil {
		return downStep{}, fmt.Errorf("migration name from schema_versions: %w", err)
	}
	if !strings.HasSuffix(name, ".sql") {
		return downStep{}, fmt.Errorf("latest applied migration %s is not a flat .sql file of %s; refusing to revert it", name, dir)
	}
	upData, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return downStep{}, fmt.Errorf("latest applied migration %s is not in %s; refusing to revert a migration outside the directory", name, dir)
	}
	if sum, _ := checksumBytes(upData); storedSum == "" || strings.TrimSpace(storedSum) != sum {
		return downStep{}, fmt.Errorf("%s in %s is not the migration that was applied (recorded checksum %q, file %s): another directory or an edited file; refusing to revert it", name, dir, storedSum, sum)
	}
	stem := strings.TrimSuffix(name, ".sql")
	cands := []string{filepath.Join(dir, stem+"_down.sql"), filepath.Join(dir, stem+".down.sql")}
	downPath := ""
	for _, c := range cands {
		if _, statErr := os.Stat(c); statErr == nil {
			downPath = c
			break
		}
	}
	if downPath == "" {
		return downStep{}, fmt.Errorf("down migration not found for %s: expected %s or %s", name, cands[0], cands[1])
	}
	data, err := os.ReadFile(downPath)
	if err != nil {
		return downStep{}, fmt.Errorf("read down migration %s: %w", downPath, err)
	}
	body, err := dirSQL(downPath, string(data), "down") // callers check err first
	return downStep{name: name, sql: body}, err
}

// downTxSQL: both ledger deletes first, then the down SQL, in dirTxSQL's transaction.
func downTxSQL(s downStep) string {
	q := strings.ReplaceAll(s.name, "'", "''")
	return dirTxSQL(s.sql, fmt.Sprintf("DELETE FROM np_common.schema_versions WHERE name = '%s';\nDELETE FROM nself_ops.migrations WHERE name = '%s';", q, q))
}
