package database

import (
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
// MigrateDownDir (`down --migration-dir [--steps N]`) and dirSQL, the
// transaction-control guard both directions share. Constraints (P7-PROD-77):
// the preview issues SELECTs only; down reverts one migration per step in one
// transaction with both ledger deletes, after every step is resolved.

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
		if _, err := dirSQL(name, string(data)); err != nil {
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

// sqlSkeleton blanks comments, quoted strings (E-string escapes included),
// quoted identifiers and dollar-quoted bodies to spaces, in one pass and
// keeping offsets, so a marker inside a string or comment hides nothing.
func sqlSkeleton(s string) string {
	out := []byte(s)
	blank := func(lo, hi int) int { // blank [lo,hi), return hi
		for k := lo; k < hi && k < len(s); k++ {
			out[k] = ' '
		}
		return hi
	}
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
			i = blank(i, past(i, "\n")-1)
		case strings.HasPrefix(s[i:], "/*"):
			i = blank(i, past(i+2, "*/"))
		case c == '\'' || c == '"':
			esc := c == '\'' && i > 0 && (s[i-1] == 'E' || s[i-1] == 'e')
			j := i + 1
			for j < len(s) && (s[j] != c || (j+1 < len(s) && s[j+1] == c)) {
				if (esc && s[j] == '\\') || s[j] == c { // escaped char or doubled quote: skip both
					j++
				}
				j++
			}
			i = blank(i, j+1)
		case c == '$' && dollarTagRe.MatchString(s[i:]):
			tag := dollarTagRe.FindString(s[i:])
			i = blank(i, past(i+len(tag), tag))
		default:
			i++
		}
	}
	return string(out)
}

var dollarTagRe = regexp.MustCompile(`^\$([A-Za-z_][A-Za-z0-9_]*)?\$`)

// txKind classifies one skeleton statement: "begin"/"end" for a plain BEGIN /
// COMMIT|END [TRANSACTION|WORK], "bad" for other transaction control (START
// TRANSACTION, ROLLBACK, ABORT, PREPARE TRANSACTION, options), else "".
func txKind(stmt string, atomic bool) string {
	w := append(strings.Fields(strings.ToLower(strings.TrimRight(stmt, ";"))), "", "") // pad: w[1], w[2] always exist
	plain := w[1] == "" || ((w[1] == "transaction" || w[1] == "work") && w[2] == "")
	switch {
	case w[0] == "begin" && w[1] != "atomic":
		return map[bool]string{true: "begin", false: "bad"}[plain]
	case w[0] == "commit", w[0] == "end" && !atomic:
		return map[bool]string{true: "end", false: "bad"}[plain]
	case w[0] == "abort", w[0] == "rollback" && w[1] != "to",
		(w[0] == "start" || w[0] == "prepare") && w[1] == "transaction":
		return "bad"
	}
	return ""
}

// dirSQL prepares a directory migration's SQL for the CLI's wrapping
// transaction: one outer BEGIN first and COMMIT last are dropped; any other
// transaction control anywhere (same line included), an unpaired BEGIN/COMMIT
// or a psql backslash command is refused, since it would end the transaction
// early and leave the ledger out of step. Non-transactional files pass as is.
func dirSQL(name, sql string) (string, error) {
	if isNonTransactional(sql) {
		return sql, nil
	}
	skel := sqlSkeleton(sql)
	refuse := func(what string) (string, error) {
		return "", fmt.Errorf("migration %s contains its own transaction control (%s); only one outer BEGIN and COMMIT are accepted, the CLI wraps each migration and its ledger update in one transaction", name, what)
	}
	if strings.Contains(skel, `\`) {
		return refuse("psql backslash command")
	}
	var spans [][2]int
	var kinds []string
	atomic := strings.Contains(strings.ToLower(skel), "begin atomic")
	for lo := 0; lo < len(skel); {
		hi := strings.IndexByte(skel[lo:], ';') + lo + 1
		if hi <= lo {
			hi = len(skel)
		}
		if strings.TrimSpace(skel[lo:hi]) != "" {
			spans = append(spans, [2]int{lo, hi})
			kinds = append(kinds, txKind(skel[lo:hi], atomic))
		}
		lo = hi
	}
	out := []byte(sql)
	for k, kind := range kinds {
		lo, hi := spans[k][0], spans[k][1]
		switch {
		case kind == "bad", kind == "begin" && k != 0, kind == "end" && k != len(kinds)-1:
			return refuse(strings.ToUpper(strings.Fields(skel[lo:hi])[0]))
		case kind != "":
			copy(out[lo:hi], strings.Repeat(" ", hi-lo))
		}
	}
	if n := len(kinds); n > 0 && (kinds[0] == "begin") != (kinds[n-1] == "end") {
		return refuse("BEGIN without COMMIT or COMMIT without BEGIN")
	}
	return string(out), nil
}

// downStep is one resolved rollback: the ledger name and its down SQL.
type downStep struct{ name, sql string }

// MigrateDownDir reverts the `steps` most recently applied migrations, which
// must all be files of dir. The ledger keys on the base name only, so the file
// in dir must hash to the checksum recorded in nself_ops.migrations for that
// name (another directory's same-named file is refused). Every step is
// resolved before the first runs; each runs its down SQL and both ledger
// deletes in one transaction. Returns the names reverted.
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
			return reverted, fmt.Errorf("revert %s: %w: %v", s.name, errs.ErrMigrationFailed, err)
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
	body, err := dirSQL(downPath, string(data)) // callers check err first
	return downStep{name: name, sql: body}, err
}

// downTxSQL: down SQL plus both ledger deletes in one transaction, with the
// timeouts MigrateUpDir uses.
func downTxSQL(s downStep) string {
	q := strings.ReplaceAll(s.name, "'", "''")
	return "BEGIN;\nSET LOCAL lock_timeout = '5s';\nSET LOCAL statement_timeout = '60s';\n" + s.sql + "\n" +
		fmt.Sprintf("DELETE FROM np_common.schema_versions WHERE name = '%s';\n", q) +
		fmt.Sprintf("DELETE FROM nself_ops.migrations WHERE name = '%s';\nCOMMIT;\n", q)
}
