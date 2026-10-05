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

// sqlSkeleton blanks comments, strings (E-strings too), quoted identifiers and
// dollar-quoted bodies to spaces in one pass, keeping offsets.
func sqlSkeleton(s string) string {
	out := []byte(s)
	skip := func(lo, hi int) int { // blank [lo,hi), return hi
		for k := lo; k < hi && k < len(s); k++ {
			out[k] = ' '
		}
		return hi
	}
	past := func(from int, tok string) int { // just past the next tok, or len(s)
		if j := strings.Index(s[from:], tok); j >= 0 {
			return from + j + len(tok)
		}
		return len(s)
	}
	ident := func(i int) bool { // s[i] is an identifier byte (t$q$, ELSE'x')
		return i >= 0 && (s[i] == '_' || s[i] == '$' || s[i] >= 0x80 || s[i]-'0' < 10 || (s[i]|32)-'a' < 26)
	}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case strings.HasPrefix(s[i:], "--"):
			i = skip(i, past(i, "\n")-1)
		case strings.HasPrefix(s[i:], "/*"):
			i = skip(i, past(i+2, "*/"))
		case c == '\'' || c == '"':
			esc := c == '\'' && i > 0 && s[i-1]|32 == 'e' && !ident(i-2)
			j := i + 1
			for j < len(s) && (s[j] != c || (j+1 < len(s) && s[j+1] == c)) {
				if (esc && s[j] == '\\') || s[j] == c { // escaped char or doubled quote: skip both
					j++
				}
				j++
			}
			i = skip(i, j+1)
		case c == '$' && !ident(i-1) && dollarTagRe.MatchString(s[i:]):
			tag := dollarTagRe.FindString(s[i:])
			i = skip(i, past(i+len(tag), tag))
		default:
			i++
		}
	}
	return string(out)
}

var dollarTagRe = regexp.MustCompile(`^\$([A-Za-z_][A-Za-z0-9_]*)?\$`)

// atomicRe: head of a SQL-standard function body (CREATE ... BEGIN ATOMIC);
// statements up to its END are not classified (Postgres rejects tx control there).
var atomicRe = regexp.MustCompile(`(?is)^\s*create\b.*\bbegin\s+atomic\b`)

// txKind classifies one skeleton statement: "begin"/"end" for a plain BEGIN /
// COMMIT|END [TRANSACTION|WORK], "bad" for other transaction control (START
// TRANSACTION, ROLLBACK, ABORT, PREPARE TRANSACTION, options), else "".
func txKind(stmt string) string {
	w := append(strings.Fields(strings.ToLower(strings.TrimRight(stmt, ";"))), "", "") // pad: w[1], w[2] exist
	plain := w[1] == "" || ((w[1] == "transaction" || w[1] == "work") && w[2] == "")
	switch {
	case w[0] == "begin":
		return map[bool]string{true: "begin", false: "bad"}[plain]
	case w[0] == "commit", w[0] == "end":
		return map[bool]string{true: "end", false: "bad"}[plain]
	case w[0] == "abort", w[0] == "rollback" && w[1] != "to",
		(w[0] == "start" || w[0] == "prepare") && w[1] == "transaction":
		return "bad"
	}
	return ""
}

// dirSQL prepares a directory migration's SQL for the CLI's transaction: one
// outer BEGIN first and COMMIT last are dropped; any other transaction control
// (same line included), an unpaired BEGIN/COMMIT or a psql backslash command is
// refused: it would end the transaction early and desync the ledger.
func dirSQL(name, sql string) (string, error) {
	skel := sqlSkeleton(sql)
	refuse := func(what string) (string, error) {
		return "", fmt.Errorf("migration %s contains its own transaction control (%s); only one outer BEGIN and COMMIT are accepted, the CLI wraps each migration and its ledger update in one transaction", name, what)
	}
	if strings.Contains(skel, `\`) {
		return refuse("psql backslash command")
	}
	var spans [][2]int
	var kinds []string
	body := false // inside a BEGIN ATOMIC function body
	for lo := 0; lo < len(skel); {
		hi := strings.IndexByte(skel[lo:], ';') + lo + 1
		if hi <= lo {
			hi = len(skel)
		}
		if st := skel[lo:hi]; strings.TrimSpace(st) != "" {
			kind := ""
			switch {
			case body:
				body = txKind(st) != "end"
			case atomicRe.MatchString(st):
				body = true
			default:
				kind = txKind(st)
			}
			spans = append(spans, [2]int{lo, hi})
			kinds = append(kinds, kind)
		}
		lo = hi
	}
	if body {
		return refuse("unterminated BEGIN ATOMIC body")
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
	if err == nil && isNonTransactional(sqlSkeleton(body)) {
		err = fmt.Errorf("down migration %s has a statement that cannot run in a transaction (CONCURRENTLY, ADD VALUE); revert it by hand", downPath)
	}
	return downStep{name: name, sql: body}, err
}

// downTxSQL: down SQL plus both ledger deletes in one transaction.
func downTxSQL(s downStep) string {
	q := strings.ReplaceAll(s.name, "'", "''")
	return "BEGIN;\nSET LOCAL lock_timeout = '5s';\nSET LOCAL statement_timeout = '60s';\n" + s.sql + "\n" +
		fmt.Sprintf("DELETE FROM np_common.schema_versions WHERE name = '%s';\n", q) +
		fmt.Sprintf("DELETE FROM nself_ops.migrations WHERE name = '%s';\nCOMMIT;\n", q)
}
