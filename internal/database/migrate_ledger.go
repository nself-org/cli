package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
)

// Purpose: migration filename validation, the MigrationStatus type, and the
// current (non-legacy) ledger bookkeeping — discovering migration files on
// disk, deriving their ledger key, and reading which ones are already applied.
// Inputs: a *config.Config and, for discovery, the on-disk migrations layout.
// Outputs: sorted file lists, ledger keys, and the applied-migrations map.
// Constraints: split out of migrate.go (CLI-R12) as a pure move; no behavior
// changed. Legacy prefix/nested-collision upgrade logic lives in
// migrate_ledger_legacy.go; SQL execution primitives live in migrate_sql.go.

// migrationNameRegex restricts migration filenames to safe characters.
// Allows letters, digits, dots, underscores, and hyphens. Prevents
// names with embedded quotes, semicolons, or control characters.
var migrationNameRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,255}$`)

// validateMigrationName fails closed on migration filenames that could
// escape a SQL string literal even after escapeSQL doubling.
func validateMigrationName(name string) error {
	if !migrationNameRegex.MatchString(name) {
		return fmt.Errorf("invalid migration name %q: only [a-zA-Z0-9._-] allowed", name)
	}
	return nil
}

// MigrationStatus describes the state of a single migration file.
type MigrationStatus struct {
	Name      string
	Applied   bool
	Timestamp time.Time
}

// migrationsDir returns the directory to scan for migration SQL files.
// If plugin is non-empty, it uses the plugin-specific migrations path.
// For non-plugin migrations, it detects the layout in order:
//  1. hasura/migrations/default/ (standard Hasura layout)
//  2. hasura/migrations/         (flat Hasura layout)
//  3. migrations/                (legacy fallback)
//  4. postgres/migrations/       (repo-local layout, e.g. ntask)
//
// If none exist on disk, it returns "hasura/migrations/default/" so error
// messages point the user at the canonical location.
func migrationsDir(cfg *config.Config, plugin string) string {
	if plugin != "" {
		pluginDir := cfg.PluginSystem.Dir
		if pluginDir == "" {
			home, _ := os.UserHomeDir()
			pluginDir = filepath.Join(home, ".nself", "plugins")
		}
		return filepath.Join(pluginDir, plugin, "migrations")
	}

	candidates := []string{
		"hasura/migrations/default",
		"hasura/migrations",
		"migrations",
		"postgres/migrations",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "hasura/migrations/default"
}

// isDownMigrationFile reports whether name (a flat-layout file basename that
// already passed the ".sql" filter) is a down/rollback file rather than an
// up migration. Matches every down-file convention seen across nself-managed
// repos: "*.down.sql" (this CLI's own `db migrate generate`) and
// "*_down.sql" (nself-org/web's convention, e.g. "017_nself_accounts_down.sql").
// Generalized to "*.down.<ext>"/"*_down.<ext>" (checked against the stripped
// extension, not a literal ".sql" suffix) so a future non-".sql" down variant
// is caught the same way, not just the two literal suffixes seen so far.
//
// WHY this exists: before this fix, only "*.down.sql" was excluded. Web's
// "*_down.sql" files were treated as ordinary up migrations and applied
// interleaved with the real ones — on production this replayed 017/018's
// down files, which DROPped tables (cli defect, verified against a prod
// pg_dump restore).
func isDownMigrationFile(name string) bool {
	ext := filepath.Ext(name)
	if ext == "" {
		return false
	}
	stem := strings.TrimSuffix(name, ext)
	lower := strings.ToLower(stem)
	return strings.HasSuffix(lower, ".down") || strings.HasSuffix(lower, "_down")
}

// scanMigrations returns sorted SQL file paths from the given directory.
// It handles two layouts:
//   - Flat: SQL files directly in dir (excludes down files — see
//     isDownMigrationFile for every recognized down-file naming convention)
//   - Nested (Hasura): subdirectories each containing an up.sql file
//
// Returns an error if the directory does not exist.
func scanMigrations(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("migrations directory not found: %s", dir)
		}
		return nil, fmt.Errorf("scan migrations dir %s: %w", dir, err)
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			// Nested layout: look for <entry>/up.sql
			upPath := filepath.Join(dir, e.Name(), "up.sql")
			if _, statErr := os.Stat(upPath); statErr == nil {
				files = append(files, upPath)
			}
		} else if strings.HasSuffix(e.Name(), ".sql") && !isDownMigrationFile(e.Name()) {
			// Flat layout: SQL file directly in dir
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}

	// Sort by ledger key, not raw basename: in the nested Hasura layout every
	// basename is "up.sql", which made the old basename sort a no-op and left
	// the apply order undefined. The key (parent dir name / filename) carries
	// the version prefix, so this yields the intended version ordering.
	sort.Slice(files, func(i, j int) bool {
		return migrationKey(files[i]) < migrationKey(files[j])
	})
	return files, nil
}

// migrationKey returns the unique ledger identity for a migration file.
// This is the value recorded in np_common.schema_versions.name and used to
// decide whether a migration has already been applied.
//
// Flat layout:   "hasura/migrations/20260701_add_users.sql" -> "20260701_add_users.sql"
// Nested layout: "hasura/migrations/default/20260701_add_users/up.sql" -> "20260701_add_users"
//
// WHY: the old code used filepath.Base(path) for both layouts, which collapsed
// every nested migration to the literal name "up.sql". Since schema_versions
// keys on name, the first nested migration recorded "up.sql" and every later
// migration was silently skipped while reporting as applied (ledger PK
// collision, Unity PCI). The parent directory name is the migration's identity
// in the Hasura layout and is unique within the migrations directory.
func migrationKey(path string) string {
	if filepath.Base(path) == "up.sql" {
		return filepath.Base(filepath.Dir(path))
	}
	return filepath.Base(path)
}

// pendingMigrationFiles returns the subset of files whose ledger key is not in
// the applied set, preserving input order. Pure function: unit-tested against
// the same-day / nested collision regressions.
func pendingMigrationFiles(files []string, applied map[string]time.Time) []string {
	var pending []string
	for _, f := range files {
		if _, ok := applied[migrationKey(f)]; !ok {
			pending = append(pending, f)
		}
	}
	return pending
}

// migrationRecordSQL builds the two ledger INSERT statements for a migration.
//
// Legacy ledger (np_common.schema_versions): plain INSERT. Keys are unique per
// migration now, so a conflict means the ledger and the gate disagree — that
// must ERROR the transaction, never silently no-op.
//
// Ops ledger (nself_ops.migrations): ON CONFLICT (id) DO UPDATE. IDs are unique
// per migration (full filename / dir name, not the date prefix), so a conflict
// can only be the SAME migration being re-recorded (e.g. forced re-run) —
// updating checksum/applied_at is "apply" semantics. The old
// ON CONFLICT (id) DO NOTHING paired with a date-prefix id silently dropped
// the ledger row of the second migration on the same day.
func migrationRecordSQL(migrationID, name, checksum string) (legacy string, ops string) {
	legacy = fmt.Sprintf("INSERT INTO np_common.schema_versions (name) VALUES ('%s');",
		strings.ReplaceAll(name, "'", "''"))
	ops = fmt.Sprintf(
		"INSERT INTO nself_ops.migrations (id, name, checksum) VALUES ('%s', '%s', '%s') "+
			"ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, checksum = EXCLUDED.checksum, applied_at = now(), rolled_back_at = NULL;",
		strings.ReplaceAll(migrationID, "'", "''"),
		strings.ReplaceAll(name, "'", "''"),
		checksum,
	)
	return legacy, ops
}

// appliedMigrations returns the set of migration names already recorded.
func appliedMigrations(ctx context.Context, cfg *config.Config) (map[string]time.Time, error) {
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}

	out, err := querySQL(ctx, cfg, db, "SELECT name || '|' || applied_at FROM np_common.schema_versions ORDER BY applied_at")
	if err != nil {
		return nil, err
	}

	result := make(map[string]time.Time)
	if out == "" {
		return result, nil
	}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		name := parts[0]
		var ts time.Time
		if len(parts) == 2 {
			ts, _ = time.Parse(time.RFC3339, parts[1])
		}
		result[name] = ts
	}
	return result, nil
}
