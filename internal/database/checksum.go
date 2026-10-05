package database

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// sha256HexRegex matches a lowercase 64-character SHA-256 hex digest.
var sha256HexRegex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// checksumBytes computes the SHA-256 hex digest of a byte slice.
func checksumBytes(data []byte) (string, error) {
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

// checksumFile computes the SHA-256 hex digest of a file.
func checksumFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file for checksum: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

// ensureOpsSchema creates the nself_ops schema if it does not exist.
func ensureOpsSchema(ctx context.Context, cfg *config.Config) error {
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}
	return runSQLOnDB(ctx, cfg, db, ensureOpsSchemaSQL)
}

// ensureMigrationsTable creates the nself_ops.migrations table if it does not exist.
// This is the S32 enhanced table with checksum, duration, and rollback tracking.
func ensureMigrationsTable(ctx context.Context, cfg *config.Config) error {
	if err := ensureOpsSchema(ctx, cfg); err != nil {
		return err
	}

	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}

	sql := ensureMigrationsSQL
	return runSQLOnDB(ctx, cfg, db, sql)
}

// ensurePromotionsTable creates the nself_ops.promotions table if it does not exist.
func ensurePromotionsTable(ctx context.Context, cfg *config.Config) error { //nolint:unused // kept: nself_ops.promotions is created by nothing and read by nothing; see qa/bugs/declared-but-never-wired-symbols.md
	if err := ensureOpsSchema(ctx, cfg); err != nil {
		return err
	}

	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}

	sql := `CREATE TABLE IF NOT EXISTS nself_ops.promotions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  from_env TEXT NOT NULL,
  to_env TEXT NOT NULL,
  approve_id TEXT,
  started_at TIMESTAMPTZ DEFAULT now(),
  finished_at TIMESTAMPTZ,
  status TEXT NOT NULL DEFAULT 'pending',
  pre_backup_tag TEXT,
  details JSONB
)`
	return runSQLOnDB(ctx, cfg, db, sql)
}

// MigrationRecord holds the state of a migration from nself_ops.migrations.
type MigrationRecord struct {
	ID         string
	Name       string
	Checksum   string
	AppliedAt  string
	AppliedBy  string
	DurationMs int
	RolledBack bool
}

// appliedMigrationsOps returns migration records from nself_ops.migrations.
func appliedMigrationsOps(ctx context.Context, cfg *config.Config) (map[string]MigrationRecord, error) {
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}

	out, err := querySQL(ctx, cfg, db,
		"SELECT id || '|' || name || '|' || checksum || '|' || COALESCE(applied_at::text,'') || '|' || COALESCE(duration_ms::text,'0') || '|' || CASE WHEN rolled_back_at IS NOT NULL THEN 'true' ELSE 'false' END FROM nself_ops.migrations ORDER BY id")
	if err != nil {
		return nil, err
	}

	result := make(map[string]MigrationRecord)
	if out == "" {
		return result, nil
	}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 6)
		if len(parts) < 6 {
			continue
		}
		rec := MigrationRecord{
			ID:         parts[0],
			Name:       parts[1],
			Checksum:   parts[2],
			AppliedAt:  parts[3],
			RolledBack: parts[5] == "true",
		}
		result[rec.ID] = rec
	}
	return result, nil
}

// VerifyChecksums compares on-disk migration checksums against stored values.
// Returns a list of mismatches (empty means all good).
func VerifyChecksums(ctx context.Context, cfg *config.Config, plugin string) ([]ChecksumMismatch, error) {
	if err := ensureSchemaVersions(ctx, cfg); err != nil {
		return nil, fmt.Errorf("ensure schema_versions: %w", err)
	}
	if err := ensureMigrationsTable(ctx, cfg); err != nil {
		return nil, fmt.Errorf("ensure migrations table: %w", err)
	}

	dir := migrationsDir(cfg, plugin)
	files, err := scanMigrations(dir)
	if err != nil {
		return nil, err
	}

	// Upgrade prefix-style ids first so lookups by the new unique id resolve.
	if err := upgradeLedger(ctx, cfg, files); err != nil {
		return nil, fmt.Errorf("upgrade migration ledger: %w", err)
	}

	applied, err := appliedMigrationsOps(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("query applied migrations: %w", err)
	}

	var mismatches []ChecksumMismatch
	for _, f := range files {
		id := extractMigrationID(f)
		rec, ok := applied[id]
		if !ok {
			continue // not applied yet, nothing to verify
		}

		diskChecksum, err := checksumFile(f)
		if err != nil {
			return nil, err
		}

		if diskChecksum != rec.Checksum {
			mismatches = append(mismatches, ChecksumMismatch{
				ID:       id,
				Name:     rec.Name,
				Expected: rec.Checksum,
				Actual:   diskChecksum,
			})
		}
	}
	return mismatches, nil
}

// ChecksumMismatch describes a migration whose on-disk checksum differs from the recorded value.
type ChecksumMismatch struct {
	ID       string
	Name     string
	Expected string // stored in DB
	Actual   string // computed from disk
}

// ResetChecksum updates the stored checksum for a migration to match the current file on disk.
func ResetChecksum(ctx context.Context, cfg *config.Config, migrationID string) error {
	if err := validateMigrationName(migrationID); err != nil {
		return fmt.Errorf("reset checksum: %w", err)
	}
	if err := ensureMigrationsTable(ctx, cfg); err != nil {
		return err
	}

	dir := migrationsDir(cfg, "")
	files, err := scanMigrations(dir)
	if err != nil {
		return err
	}

	for _, f := range files {
		if extractMigrationID(f) == migrationID {
			cs, err := checksumFile(f)
			if err != nil {
				return err
			}
			// cs is SHA-256 hex; enforce defensively before use.
			if !sha256HexRegex.MatchString(cs) {
				return fmt.Errorf("unexpected checksum format for %s", migrationID)
			}
			// Use a parameterized query to avoid any SQL injection risk.
			conn, err := Open(ctx, cfg)
			if err != nil {
				return fmt.Errorf("open database for checksum update: %w", err)
			}
			defer func() { _ = conn.Close() }()
			_, err = conn.ExecContext(ctx,
				"UPDATE nself_ops.migrations SET checksum = $1 WHERE id = $2",
				cs, migrationID)
			return err
		}
	}
	return fmt.Errorf("migration %s not found on disk", migrationID)
}

// extractMigrationID derives the unique nself_ops.migrations id for a
// migration file.
//
// Flat layout:   "20260701_add_users.sql"        -> "20260701_add_users"
// Nested layout: "20260701_add_users/up.sql"     -> "20260701_add_users"
//
// WHY full name, not the timestamp prefix: the old implementation truncated
// at the first underscore, so two migrations sharing a date-derived version
// (e.g. 20260701_add_users.sql and 20260701_add_orders.sql) collided on the
// ledger PRIMARY KEY and the second row was silently dropped by
// ON CONFLICT (id) DO NOTHING (Unity PCI, "ledger outlives DDL" family).
// The full name keeps same-day migrations distinct. Existing ledgers with
// prefix-style ids are upgraded in place by upgradeLedger.
func extractMigrationID(path string) string {
	if filepath.Base(path) == "up.sql" {
		return filepath.Base(filepath.Dir(path))
	}
	base := strings.TrimSuffix(filepath.Base(path), ".sql")
	return strings.TrimSuffix(base, ".down")
}
