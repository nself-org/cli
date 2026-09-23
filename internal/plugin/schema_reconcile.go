package plugin

// schema_reconcile.go — reconciles np_common.schema_versions with a
// pre-existing, differently-shaped table, split out of schema.go for file
// size (repoqa 300-line cap).
//
// Purpose: np_common.schema_versions is used by TWO unrelated systems under
//          the same name: this package's per-plugin version tracking
//          (plugin TEXT, version INT, applied_at ...) and, independently,
//          internal/database's migration ledger (name TEXT [PRIMARY KEY],
//          applied_at ...). Whichever one creates the table first wins the
//          shape; reconcileSchemaVersionsTable brings a table created by the
//          other system up to the shape this package needs, in place,
//          without touching the rows or columns the other system owns.
// Inputs:  a *config.Config identifying the target Postgres container.
// Outputs: np_common.schema_versions carries usable "plugin" and "version"
//          columns on return, with any legacy rows preserved and backfilled
//          where the rule below applies; error on a failed DDL/DML step.
// Constraints: additive/relaxing only (CREATE SCHEMA/TABLE IF NOT EXISTS,
//              ADD COLUMN IF NOT EXISTS, DROP NOT NULL on a pre-existing
//              "name" column) — never DROP, never touches existing rows
//              beyond the single documented UPDATE backfill, never narrows
//              or removes anything the migration ledger relies on.
// SPORT: plugin-schema; callers: createPluginSchema (schema.go)

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// schemaVersionsColumn describes one column of np_common.schema_versions as
// reconcileSchemaVersionsTable found it.
type schemaVersionsColumn struct {
	Nullable bool
}

// schemaVersionsColumns returns the columns currently present on
// np_common.schema_versions, keyed by column name. Used by
// reconcileSchemaVersionsTable to decide which columns are missing, and
// whether a pre-existing "name" column still carries a NOT NULL constraint
// that would reject a plugin-only row.
func schemaVersionsColumns(ctx context.Context, cfg *config.Config) (map[string]schemaVersionsColumn, error) {
	out, err := queryPSQL(ctx, cfg,
		`SELECT column_name || '|' || is_nullable FROM information_schema.columns WHERE table_schema = 'np_common' AND table_name = 'schema_versions';`,
	)
	if err != nil {
		return nil, err
	}
	cols := make(map[string]schemaVersionsColumn)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		cols[parts[0]] = schemaVersionsColumn{Nullable: parts[1] == "YES"}
	}
	return cols, nil
}

// reconcileSchemaVersionsTable ensures np_common.schema_versions carries
// usable "plugin" and "version" columns for getSchemaVersion/
// recordSchemaVersion, regardless of what created the table first.
//
// np_common.schema_versions is also the migration ledger table
// (internal/database's ensureSchemaVersions, an unrelated, older system):
// it creates the SAME table name with a DIFFERENT shape — "name TEXT
// [PRIMARY KEY], applied_at ..." — and no "plugin"/"version" columns. On any
// project where migrations ran before the first schema-owning plugin was
// installed (true of nself's own production host), createPluginSchema's old
// `CREATE TABLE IF NOT EXISTS` was a silent no-op against that pre-existing
// table: getSchemaVersion's SELECT and recordSchemaVersion's INSERT both
// referenced a "plugin" column that did not exist. getSchemaVersion masked
// this (any query error there is treated as "no version recorded yet"), so
// the failure only surfaced downstream at the INSERT: "nself plugin update
// cron" on production (nself 1.4.8, 2026-09-23) failed with
// `column "plugin" of relation "schema_versions" does not exist`.
//
// Adding the columns alone is not enough: the legacy table also declares
// "name TEXT NOT NULL" with no default, and a plugin-only row never sets
// "name". A live-Postgres run against that exact shape confirmed the
// resulting failure — `null value in column "name" ... violates not-null
// constraint`, `Failing row contains (null, <ts>, np_cron, 1)` — so this
// also relaxes that one constraint (DROP NOT NULL) when a legacy "name"
// column is found still requiring it. The migration ledger's own INSERT
// always supplies "name" regardless, so its behavior is unchanged; only a
// row it never writes (a plugin-only row) is newly allowed to leave "name"
// unset.
//
// It is idempotent and safe to call before every schema operation.
func reconcileSchemaVersionsTable(ctx context.Context, cfg *config.Config) error {
	createSQL := `CREATE SCHEMA IF NOT EXISTS np_common;
CREATE TABLE IF NOT EXISTS np_common.schema_versions (
  plugin TEXT,
  version INT,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`
	if err := execPSQL(ctx, cfg, createSQL); err != nil {
		return fmt.Errorf("creating np_common.schema_versions: %w", err)
	}

	cols, err := schemaVersionsColumns(ctx, cfg)
	if err != nil {
		return fmt.Errorf("reading np_common.schema_versions columns: %w", err)
	}

	if err := reconcileVersionColumns(ctx, cfg, cols); err != nil {
		return err
	}

	addUniqueIndexIfSafe(ctx, cfg)
	return nil
}

// reconcileVersionColumns adds any missing "plugin"/"version" columns,
// backfills "plugin" from a legacy "name" column, and relaxes a legacy
// "name NOT NULL" constraint so a plugin-only insert can leave it unset.
// Split out of reconcileSchemaVersionsTable to keep that function under the
// repo's per-function line limit.
func reconcileVersionColumns(ctx context.Context, cfg *config.Config, cols map[string]schemaVersionsColumn) error {
	if _, ok := cols["plugin"]; !ok {
		if err := execPSQL(ctx, cfg, `ALTER TABLE np_common.schema_versions ADD COLUMN IF NOT EXISTS plugin TEXT;`); err != nil {
			return fmt.Errorf("adding plugin column to np_common.schema_versions: %w", err)
		}
	}
	if _, ok := cols["version"]; !ok {
		if err := execPSQL(ctx, cfg, `ALTER TABLE np_common.schema_versions ADD COLUMN IF NOT EXISTS version INT;`); err != nil {
			return fmt.Errorf("adding version column to np_common.schema_versions: %w", err)
		}
	}

	name, hasName := cols["name"]
	if !hasName {
		return nil
	}

	if !name.Nullable {
		if err := execPSQL(ctx, cfg, `ALTER TABLE np_common.schema_versions ALTER COLUMN name DROP NOT NULL;`); err != nil {
			return fmt.Errorf("relaxing legacy name NOT NULL on np_common.schema_versions: %w", err)
		}
	}

	// Backfill rule: the legacy migration-ledger shape keys rows by "name",
	// which is the same logical identity this package tracks as "plugin" —
	// both answer "what does this row belong to". Only rows this package has
	// never written (plugin IS NULL) are touched, so a rerun never clobbers
	// a value this package already recorded.
	backfillSQL := `UPDATE np_common.schema_versions SET plugin = name WHERE plugin IS NULL AND name IS NOT NULL;`
	if err := execPSQL(ctx, cfg, backfillSQL); err != nil {
		return fmt.Errorf("backfilling plugin column in np_common.schema_versions: %w", err)
	}
	return nil
}

// addUniqueIndexIfSafe adds a unique index on (plugin, version) so the table
// stays well-formed going forward. It is skipped if any existing rows would
// violate it; recordSchemaVersion does not depend on this index (it uses a
// WHERE NOT EXISTS guard), so a skip here only means a missed optimization,
// never a correctness gap. Legacy rows backfilled by reconcile have
// version = NULL, and Postgres unique indexes never treat NULL as equal to
// NULL, so they cannot trigger the duplicate check below. Errors checking
// for duplicates or creating the index are non-fatal for the same reason.
func addUniqueIndexIfSafe(ctx context.Context, cfg *config.Config) {
	dupCount, err := queryPSQL(ctx, cfg, `SELECT COUNT(*) FROM (
  SELECT plugin, version FROM np_common.schema_versions
  WHERE plugin IS NOT NULL AND version IS NOT NULL
  GROUP BY plugin, version HAVING COUNT(*) > 1
) dup;`)
	if err != nil || dupCount != "0" {
		return
	}
	indexSQL := `CREATE UNIQUE INDEX IF NOT EXISTS schema_versions_plugin_version_idx ON np_common.schema_versions (plugin, version);`
	_ = execPSQL(ctx, cfg, indexSQL)
}
