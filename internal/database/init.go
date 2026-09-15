package database

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// waitForPostgres and its readiness-streak machinery live in readiness.go —
// see postgresReadinessStreak for why a stable streak is required instead of
// a first-success check.

// runSQLOnDB executes a SQL statement inside the postgres container via psql,
// targeting a specific database. This is needed for init operations that must
// connect to the "postgres" admin database (e.g., CREATE DATABASE).
// When cfg.EmbeddedPG is true, the statement is executed against the pglite UDS
// instance instead (pipeSQLEmbedded handles the UDS connection).
func runSQLOnDB(ctx context.Context, cfg *config.Config, database string, sql string) error {
	if cfg.EmbeddedPG {
		// Embedded pglite: route through pipeSQLEmbedded (defined in migrate.go).
		// The "database" parameter is ignored — pglite boots a single DB.
		return pipeSQLEmbedded(ctx, cfg.EmbeddedPGDatabaseURL(embeddedPGRuntimeDir(cfg)), sql)
	}

	container := containerName(cfg)
	user := cfg.Postgres.User
	if user == "" {
		user = "postgres"
	}

	// Wrapped in retryTransientPG: during nself start's init phase this can
	// still land in the postgres image's temporary-server shutdown window
	// even after waitForPostgres reports ready (a slow enough shutdown
	// catches the NEXT statement, not just the wait). Genuine failures
	// (auth, bad DSN, etc.) are not transient and return immediately.
	return retryTransientPG(ctx, func() error {
		var stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, "docker", "exec", container,
			"psql", "-U", user, "-d", database, "-c", sql,
		)
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("psql exec failed: %s: %w", strings.TrimSpace(stderr.String()), err)
		}
		return nil
	})
}

// createDatabase creates the target database if it does not already exist.
// It connects to the default "postgres" database to run the CREATE DATABASE.
func createDatabase(ctx context.Context, cfg *config.Config) error {
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}

	container := containerName(cfg)
	user := cfg.Postgres.User
	if user == "" {
		user = "postgres"
	}

	// Validate database name before any SQL interpolation.
	// Note: pg_database.datname check uses a string literal (safe to
	// embed as a single-quoted literal after escape), but we still
	// validate to fail fast on invalid names.
	if _, err := SanitizeIdentifier(db); err != nil {
		return fmt.Errorf("invalid database name in config: %w", err)
	}
	// Check if database already exists. The :'varname' substitution does NOT
	// fire when psql is invoked via -c/-tAc — the literal reaches the server
	// and breaks. Inline the validated identifier instead. Safe because db has
	// already passed SanitizeIdentifier above.
	checkSQL := fmt.Sprintf("SELECT 1 FROM pg_database WHERE datname = '%s'", db)

	// See run 32948887818: this exact query is what landed in the postgres
	// image's temporary-server shutdown window on a clean first run, right
	// after waitForPostgres reported ready. Wrapped in retryTransientPG so
	// that race no longer surfaces as a hard failure; non-transient errors
	// (auth, bad role, etc.) still return immediately with their real message.
	var stdout bytes.Buffer
	err := retryTransientPG(ctx, func() error {
		stdout.Reset()
		var stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, "docker", "exec", container,
			"psql", "-U", user, "-d", "postgres", "-tAc", checkSQL,
		)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if runErr := cmd.Run(); runErr != nil {
			return fmt.Errorf("%s: %w", strings.TrimSpace(stderr.String()), runErr)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("check database existence: %w", err)
	}

	if strings.TrimSpace(stdout.String()) == "1" {
		slog.Info("database already exists", "database", db)
		return nil
	}

	// Database does not exist; create it. Identifier already validated above.
	quotedDB, err := SanitizeIdentifier(db)
	if err != nil {
		return fmt.Errorf("sanitize database name: %w", err)
	}
	createSQL := fmt.Sprintf("CREATE DATABASE %s", quotedDB)
	if err := runSQLOnDB(ctx, cfg, "postgres", createSQL); err != nil {
		// "already exists" here is success, not failure.
		//
		// The existence check above and this CREATE can see two different
		// servers. The postgres image runs a TEMPORARY server to execute its
		// entrypoint scripts, then shuts it down and starts the real one. The
		// check can land on the temporary server, where the database genuinely
		// does not exist yet, while POSTGRES_DB has already created it on the
		// real server by the time CREATE runs:
		//
		//   Error: database init: create database testproject:
		//   psql exec failed: ERROR: database "testproject" already exists
		//
		// That killed `nself start` at step 5 of the golden path on a clean
		// first run. The comment above already records this window biting the
		// check; the check was wrapped in retryTransientPG and this call was
		// not.
		//
		// Retrying cannot help — the database exists and will keep existing.
		// The function's contract is "create it if it does not already exist",
		// so the only correct response to finding it already there is to carry
		// on. Any other error still fails.
		if isDatabaseAlreadyExists(err) {
			slog.Info("database already exists (created concurrently by the postgres image)", "database", db)
			return nil
		}
		return fmt.Errorf("create database %s: %w", db, err)
	}

	slog.Info("database created", "database", db)
	return nil
}

// createSchemas creates the required schemas (auth, storage, public) in the
// target database and grants all privileges to the configured user.
func createSchemas(ctx context.Context, cfg *config.Config) error {
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}
	user := cfg.Postgres.User
	if user == "" {
		user = "postgres"
	}

	schemas := []string{"auth", "storage", "public"}

	// Validate user before interpolation. Schemas are compile-time constants.
	quotedUser, err := SanitizeIdentifier(user)
	if err != nil {
		return fmt.Errorf("invalid postgres user name: %w", err)
	}

	for _, schema := range schemas {
		quotedSchema := MustSanitizeIdentifier(schema)
		createSQL := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", quotedSchema)
		if err := runSQLOnDB(ctx, cfg, db, createSQL); err != nil {
			return fmt.Errorf("create schema %s: %w", schema, err)
		}

		grantSQL := fmt.Sprintf("GRANT ALL ON SCHEMA %s TO %s", quotedSchema, quotedUser)
		if err := runSQLOnDB(ctx, cfg, db, grantSQL); err != nil {
			return fmt.Errorf("grant schema %s to %s: %w", schema, user, err)
		}
	}

	slog.Info("schemas created and granted", "schemas", schemas, "user", user)
	return nil
}

// createExtensions installs the required PostgreSQL extensions (pgcrypto, citext)
// in the target database. Note: uuid-ossp is NOT auto-created.
func createExtensions(ctx context.Context, cfg *config.Config) error {
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}

	extensions := []string{"pgcrypto", "citext"}

	for _, ext := range extensions {
		quotedExt := MustSanitizeIdentifier(ext)
		sql := fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %s", quotedExt)
		if err := runSQLOnDB(ctx, cfg, db, sql); err != nil {
			return fmt.Errorf("create extension %s: %w", ext, err)
		}
	}

	slog.Info("extensions created", "extensions", extensions)
	return nil
}

// InitializeDatabase waits for PostgreSQL to become ready, then creates the
// database, schemas, grants, and required extensions. This is Phase 3 of the
// nself startup sequence.
//
// Steps:
//  1. Wait for a stable streak of successful readiness probes (max 60s, see
//     postgresReadinessStreak)
//  2. CREATE DATABASE IF NOT EXISTS
//  3. CREATE SCHEMA IF NOT EXISTS auth, storage, public
//  4. GRANT ALL ON SCHEMA auth, storage, public TO user
//  5. CREATE EXTENSION IF NOT EXISTS pgcrypto, citext
//  6. Guarantee public.uuidv7() exists (extension when available, SQL otherwise)
func InitializeDatabase(ctx context.Context, cfg *config.Config) error {
	slog.Info("initializing database")

	// When the embedded PG runtime is active, the postgres socket is already
	// ready (the runtime's Start() waits for the socket before returning), so
	// we skip the container-based waitForPostgres / createDatabase / createSchemas /
	// createExtensions steps that rely on `docker exec psql`.
	//
	// The embedded pglite runtime boots with an empty database that already
	// supports pgvector. Schemas and extensions that the stack needs at startup
	// are applied by Hasura on first connection (catalog tracking) and by the
	// migration runner via the UDS DSN (cfg.EmbeddedPGDatabaseURL). No separate
	// Docker container initialization is required.
	if cfg.EmbeddedPG {
		slog.Info("embedded PG active — skipping container-based database initialization")
		return nil
	}

	if err := waitForPostgres(ctx, cfg); err != nil {
		return err
	}

	if err := createDatabase(ctx, cfg); err != nil {
		return err
	}

	if err := createSchemas(ctx, cfg); err != nil {
		return err
	}

	if err := createExtensions(ctx, cfg); err != nil {
		return err
	}

	if err := ensureUUIDv7(ctx, cfg); err != nil {
		return err
	}

	slog.Info("database initialization complete")
	return nil
}

// isDatabaseAlreadyExists reports whether err is Postgres telling us the
// database is already there.
//
// Matched on message text rather than SQLSTATE because the command runs via
// `docker exec ... psql`, which surfaces the server error as stderr text and an
// exit status; there is no structured error to read a 42P04 out of. The message
// is stable across every supported Postgres major.
//
// Deliberately narrow: it must not swallow "permission denied to create
// database" or a connection failure, which are real and must still fail.
func isDatabaseAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") &&
		strings.Contains(msg, "database")
}
