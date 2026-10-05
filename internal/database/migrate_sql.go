package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"

	// pgx stdlib registers the "pgx" driver used by the embedded-PG SQL path.
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
)

// Purpose: low-level SQL execution primitives (embedded pglite + container
// psql) and SQL-text analysis helpers used by the migration runner.
// Inputs: a context, the resolved *config.Config, and raw SQL text.
// Outputs: query results as strings, or an error from the execution path.
// Constraints: split out of migrate.go (CLI-R12) as a pure move; no behavior
// changed. Keep in sync with migrate.go / migrate_ledger.go, which call into
// these helpers for every migration apply/revert.

// querySQLEmbedded executes a query against the embedded pglite instance via its
// Unix-domain socket. Returns the first column of the first row as a
// newline-joined string (mirrors the `-tAc` psql output format).
func querySQLEmbedded(ctx context.Context, dsn string, query string) (string, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return "", fmt.Errorf("embedded sql.Open: %w", err)
	}
	defer db.Close() //nolint:errcheck

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return "", fmt.Errorf("embedded query: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var lines []string
	for rows.Next() {
		var val string
		if scanErr := rows.Scan(&val); scanErr != nil {
			return "", fmt.Errorf("embedded row scan: %w", scanErr)
		}
		lines = append(lines, val)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("embedded rows: %w", err)
	}
	return strings.Join(lines, "\n"), nil
}

// pipeSQLEmbedded executes raw SQL text against the embedded pglite instance
// via its Unix-domain socket. Mirrors pipeSQLToContainer for the embedded path.
func pipeSQLEmbedded(ctx context.Context, dsn string, sqlText string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("embedded sql.Open: %w", err)
	}
	defer db.Close() //nolint:errcheck

	if _, err := db.ExecContext(ctx, sqlText); err != nil {
		return fmt.Errorf("embedded exec: %w", err)
	}
	return nil
}

// querySQL executes a SQL query inside the postgres container and returns stdout.
// Unlike runSQL from init.go, this captures and returns the output text.
// When cfg.EmbeddedPG is true, the query is routed through the pglite UDS instead.
func querySQL(ctx context.Context, cfg *config.Config, database string, sqlText string) (string, error) {
	if cfg.EmbeddedPG {
		dsn := cfg.EmbeddedPGDatabaseURL(embeddedPGRuntimeDir(cfg))
		return querySQLEmbedded(ctx, dsn, sqlText)
	}

	container := containerName(cfg)
	user := cfg.Postgres.User
	if user == "" {
		user = "postgres"
	}

	stdout, stderr, err := docker.ExecStdin(ctx, container,
		[]string{"psql", "-U", user, "-d", database, "-tAc", sqlText}, nil)
	if err != nil {
		return "", fmt.Errorf("psql: %s: %w", strings.TrimSpace(stderr), err)
	}
	return strings.TrimSpace(stdout), nil
}

// pipeSQLToContainer pipes raw SQL text into psql inside the postgres container.
// When cfg.EmbeddedPG is true, the SQL is executed against the pglite UDS instead.
func pipeSQLToContainer(ctx context.Context, cfg *config.Config, sqlText string) error {
	if cfg.EmbeddedPG {
		dsn := cfg.EmbeddedPGDatabaseURL(embeddedPGRuntimeDir(cfg))
		return pipeSQLEmbedded(ctx, dsn, sqlText)
	}

	container := containerName(cfg)
	user := cfg.Postgres.User
	if user == "" {
		user = "postgres"
	}
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}

	_, stderr, err := docker.ExecStdin(ctx, container, []string{
		"psql", "-U", user, "-d", db, "-v", "ON_ERROR_STOP=1",
	}, strings.NewReader(sqlText))
	if err != nil {
		return fmt.Errorf("psql: %s: %w", strings.TrimSpace(stderr), err)
	}
	return nil
}

// execSQLArg runs sqlText through `psql -c` (no stdin, so it is not a piped
// migration run) against cfg's database; embedded PG goes through the UDS.
func execSQLArg(ctx context.Context, cfg *config.Config, sqlText string) error {
	if cfg.EmbeddedPG {
		return pipeSQLEmbedded(ctx, cfg.EmbeddedPGDatabaseURL(embeddedPGRuntimeDir(cfg)), sqlText)
	}
	user := cfg.Postgres.User
	if user == "" {
		user = "postgres"
	}
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}
	_, stderr, err := docker.ExecStdin(ctx, containerName(cfg),
		[]string{"psql", "-U", user, "-d", db, "-v", "ON_ERROR_STOP=1", "-c", sqlText}, nil)
	if err != nil {
		return fmt.Errorf("psql exec failed: %s: %w", strings.TrimSpace(stderr), err)
	}
	return nil
}

// embeddedPGRuntimeDir returns the runtime directory used by the pglite/wasmtime
// embedded runtime. It reads NSELF_RUNTIME_DIR first (set by `nself start --embedded-pg`)
// then falls back to the project-local default.
func embeddedPGRuntimeDir(cfg *config.Config) string {
	if dir := os.Getenv("NSELF_RUNTIME_DIR"); dir != "" {
		return dir
	}
	// Fall back to project-local default: $PWD/.nself/embedded-pg
	wd, _ := os.Getwd()
	return wd + "/.nself/embedded-pg"
}

// ensureSchemaVersions creates the np_common.schema_versions table if it does not exist.
func ensureSchemaVersions(ctx context.Context, cfg *config.Config) error {
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}

	sql := `CREATE SCHEMA IF NOT EXISTS np_common; CREATE TABLE IF NOT EXISTS np_common.schema_versions (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`
	return runSQLOnDB(ctx, cfg, db, sql)
}

// isNonTransactional checks if a migration SQL contains statements that cannot
// run inside a transaction (e.g., CREATE INDEX CONCURRENTLY).
// Note: ALTER TYPE ... ADD VALUE requires non-transactional execution in PostgreSQL 16;
// other ALTER TYPE forms (RENAME, DROP ATTRIBUTE, ADD ATTRIBUTE) are fully transactional.
func isNonTransactional(sql string) bool {
	upper := strings.ToUpper(sql)
	if strings.Contains(upper, "CREATE INDEX CONCURRENTLY") ||
		strings.Contains(upper, "DROP INDEX CONCURRENTLY") ||
		strings.Contains(upper, "REINDEX CONCURRENTLY") {
		return true
	}
	// Only `ALTER TYPE ... ADD VALUE` cannot run in a transaction. Other ALTER
	// TYPE forms (RENAME, OWNER TO, SET SCHEMA) are transaction-safe and must
	// keep their atomicity, so do not match those.
	return alterTypeAddValueRegex.MatchString(upper)
}

// alterTypeAddValueRegex matches `ALTER TYPE ... ADD VALUE`, the only ALTER TYPE
// form that PostgreSQL refuses to run inside a transaction block.
var alterTypeAddValueRegex = regexp.MustCompile(`ALTER\s+TYPE\b[\s\S]*?\bADD\s+VALUE\b`)

// statementTimeoutDirective lets a migration opt out of (or change) the default
// 60s statement_timeout, e.g. for large data backfills:
//
//	-- nself:statement-timeout=0       (disable; no limit)
//	-- nself:statement-timeout=600s    (raise to 10 minutes)
var statementTimeoutDirective = regexp.MustCompile(`(?i)--\s*nself:statement-timeout\s*=\s*([0-9]+[a-z]*)`)

// statementTimeoutFor returns the statement_timeout value a migration should
// use. Defaults to "60s"; a migration may override it via the directive comment.
func statementTimeoutFor(sql string) string {
	if m := statementTimeoutDirective.FindStringSubmatch(sql); m != nil {
		return m[1]
	}
	return "60s"
}
