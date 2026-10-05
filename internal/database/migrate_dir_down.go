package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// Purpose: PendingDirMigrations (read-only preview for `up --migration-dir
// --dry-run`), MigrateDownDir (`down --migration-dir [--steps N]`) and the
// transaction-control guard both directions share.
// Inputs: a *config.Config, a migrations directory, and (down) a step count.
// Outputs: the ledger names that are pending / were reverted, or an error.
// Constraints (P7-PROD-77): the preview issues SELECTs only. Down reverts one
// migration per step in one transaction with both ledger deletes, after every
// step is resolved. A file with its own transaction control is refused: it
// would end the wrapping transaction and leave the ledger out of step.

// opsChecksums reads name -> stored checksum from nself_ops.migrations
// (read-only); an absent table yields an empty map.
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
		if p := strings.SplitN(strings.TrimSpace(line), "|", 2); len(p) == 2 {
			sums[p[0]] = p[1]
		}
	}
	return sums, nil
}

// PendingDirMigrations lists the files in dir that MigrateUpDir would apply,
// without writing anything. It applies the same decisions as MigrateUpDir and
// ApplyFile: ledger name = file base name, skip when recorded in
// np_common.schema_versions, fail on a checksum mismatch against
// nself_ops.migrations, and refuse what the real run refuses (SQL lint,
// transaction control, ALTER prerequisites). Missing ledger tables mean
// "nothing applied yet" (the real run would create them); none is created here.
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
		if err := validateMigrationName(name); err != nil {
			return nil, err
		}
		data, readErr := os.ReadFile(f)
		if readErr != nil {
			return nil, fmt.Errorf("read migration file %s: %w", name, readErr)
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
		if err := rejectTxControl(name, string(data)); err != nil {
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

// sqlSkeleton returns sql with comments, quoted strings (E” escapes
// included), quoted identifiers and dollar-quoted bodies each replaced by one
// space, so what remains is only statement structure. One pass, so a comment
// marker inside a string (or a quote inside a comment) cannot hide anything.
func sqlSkeleton(s string) string {
	var b strings.Builder
	past := func(from int, end string) int { // index just past the next end, or len(s)
		if j := strings.Index(s[from:], end); j >= 0 {
			return from + j + len(end)
		}
		return len(s)
	}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case strings.HasPrefix(s[i:], "--"):
			i = past(i, "\n")
			b.WriteByte(' ')
		case strings.HasPrefix(s[i:], "/*"):
			i = past(i+2, "*/")
			b.WriteByte(' ')
		case c == '\'' || c == '"':
			esc := c == '\'' && i > 0 && (s[i-1] == 'E' || s[i-1] == 'e')
			j := i + 1
			for j < len(s) && (s[j] != c || (j+1 < len(s) && s[j+1] == c)) {
				if (esc && s[j] == '\\') || s[j] == c { // escaped char or doubled quote: skip both
					j++
				}
				j++
			}
			i = j + 1
			b.WriteByte(' ')
		case c == '$' && dollarTagRe.MatchString(s[i:]):
			tag := dollarTagRe.FindString(s[i:])
			i = past(i+len(tag), tag)
			b.WriteByte(' ')
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

var dollarTagRe = regexp.MustCompile(`^\$([A-Za-z_][A-Za-z0-9_]*)?\$`)

// rejectTxControl refuses SQL that would break the transaction a directory
// migration is wrapped in: BEGIN/START TRANSACTION, COMMIT, END, ROLLBACK
// (not ROLLBACK TO), ABORT or PREPARE TRANSACTION as a statement anywhere in
// the stream, and any backslash outside literals (a psql meta-command such as
// \set AUTOCOMMIT). Files that run outside a transaction (CREATE INDEX
// CONCURRENTLY) are exempt: they are not wrapped.
func rejectTxControl(name, sql string) error {
	if isNonTransactional(sql) {
		return nil
	}
	skel := sqlSkeleton(sql)
	bad := ""
	if strings.Contains(skel, `\`) {
		bad = `a backslash (psql meta-command)`
	}
	atomic := strings.Contains(strings.ToLower(skel), "begin atomic")
	for _, stmt := range strings.Split(skel, ";") {
		w := strings.Fields(strings.ToLower(stmt))
		if len(w) == 0 {
			continue
		}
		switch {
		case w[0] == "commit", w[0] == "abort", w[0] == "start" && len(w) > 1 && w[1] == "transaction",
			w[0] == "prepare" && len(w) > 1 && w[1] == "transaction",
			w[0] == "rollback" && !(len(w) > 1 && w[1] == "to"),
			w[0] == "end" && !atomic,
			w[0] == "begin" && !(len(w) > 1 && w[1] == "atomic"):
			bad = strings.ToUpper(w[0])
		}
	}
	if bad != "" {
		return fmt.Errorf("migration %s contains its own transaction control (%s); remove it, the CLI wraps each migration and its ledger update in one transaction", name, bad)
	}
	return nil
}

// downStep is one resolved rollback: the ledger name and its down SQL.
type downStep struct {
	name string
	sql  string
}

// resolveDownFile finds <name>_down.sql or <name>.down.sql in dir for the
// flat migration "<name>.sql". A missing file is an error naming both paths.
func resolveDownFile(dir, ledgerName string) (string, error) {
	stem := strings.TrimSuffix(ledgerName, ".sql")
	candidates := []string{filepath.Join(dir, stem+"_down.sql"), filepath.Join(dir, stem+".down.sql")}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("down migration not found for %s: expected %s or %s", ledgerName, candidates[0], candidates[1])
}

// MigrateDownDir reverts the `steps` most recently applied migrations, which
// must all be files of dir. The ledger keys on the base name only, so a file
// of the same name in another directory must not be mistaken for this one:
// the file in dir must hash to the checksum recorded in nself_ops.migrations
// for that name, or nothing is reverted. If the newest applied entry is not a
// matching file of dir, or its down file is missing, nothing is reverted and
// the error names it. Each step runs its down SQL and both ledger deletes in
// one transaction, so a failing step leaves its own ledger rows in place.
// Returns the names reverted.
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

// planDownStep validates one ledger name against dir (including that the file
// there is the one that was applied, by checksum) and reads its down SQL.
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
	downPath, err := resolveDownFile(dir, name)
	if err != nil {
		return downStep{}, err
	}
	data, err := os.ReadFile(downPath)
	if err != nil {
		return downStep{}, fmt.Errorf("read down migration %s: %w", downPath, err)
	}
	if err := rejectTxControl(downPath, string(data)); err != nil {
		return downStep{}, err
	}
	return downStep{name: name, sql: string(data)}, nil
}

// downTxSQL wraps the down SQL and both ledger deletes in one transaction
// (same shape as MigrateDown, plus the lock/statement timeouts MigrateUpDir
// uses so a stuck rollback aborts instead of blocking production).
func downTxSQL(s downStep) string {
	q := strings.ReplaceAll(s.name, "'", "''")
	return "BEGIN;\nSET LOCAL lock_timeout = '5s';\nSET LOCAL statement_timeout = '60s';\n" + s.sql + "\n" +
		fmt.Sprintf("DELETE FROM np_common.schema_versions WHERE name = '%s';\n", q) +
		fmt.Sprintf("DELETE FROM nself_ops.migrations WHERE name = '%s';\nCOMMIT;\n", q)
}
