package plugin

// schema_reconcile.go — owns np_common.plugin_schema_versions, the table this
// package records per-plugin schema versions in, split out of schema.go for
// file size (repoqa 300-line cap).
//
// Purpose: this package used to record plugin versions in
//          np_common.schema_versions, a name it shares with
//          internal/database's migration ledger (name TEXT PRIMARY KEY,
//          applied_at ...). Whichever system created the table first won the
//          shape, and the other broke: on nself's own production host the
//          ledger won, "nself plugin update cron" failed on a missing
//          "plugin" column (1.4.8), and the 1.4.9 in-place reconcile then
//          failed on "column name is in a primary key" (2026-09-23). Plugin
//          versions now live in their own table; the ledger table is never
//          created, altered or written by this package again.
// Inputs:  a *config.Config identifying the target Postgres container.
// Outputs: np_common.plugin_schema_versions exists on return, carrying any
//          plugin rows an earlier CLI recorded in np_common.schema_versions.
// Constraints: additive only (CREATE ... IF NOT EXISTS, INSERT ... ON
//              CONFLICT DO NOTHING); reads np_common.schema_versions but
//              never writes it.
// SPORT: plugin-schema; callers: createPluginSchema (schema.go)

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// pluginSchemaVersionsDDL creates the dedicated tracking table. The composite
// primary key makes recordSchemaVersion's ON CONFLICT DO NOTHING exact.
const pluginSchemaVersionsDDL = `CREATE SCHEMA IF NOT EXISTS np_common;
CREATE TABLE IF NOT EXISTS np_common.plugin_schema_versions (
  plugin TEXT NOT NULL,
  version INT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (plugin, version)
);`

// ensurePluginSchemaVersionsTable creates np_common.plugin_schema_versions
// and carries over plugin rows from np_common.schema_versions, so a plugin
// already recorded by an older CLI is not re-provisioned. It is idempotent
// and safe to call before every schema operation.
func ensurePluginSchemaVersionsTable(ctx context.Context, cfg *config.Config) error {
	if err := execPSQL(ctx, cfg, pluginSchemaVersionsDDL); err != nil {
		return fmt.Errorf("creating np_common.plugin_schema_versions: %w", err)
	}
	return carryOverLegacyPluginRows(ctx, cfg)
}

// carryOverLegacyPluginRows copies (plugin, version) rows from
// np_common.schema_versions when that table has both columns. Rows without a
// version are migration-ledger rows (or the 1.4.9 plugin = name backfill of
// them) and are skipped. A missing table or column means there is nothing to
// carry over, not an error.
func carryOverLegacyPluginRows(ctx context.Context, cfg *config.Config) error {
	cols, err := queryPSQL(ctx, cfg, `SELECT string_agg(column_name, ',' ORDER BY column_name)
FROM information_schema.columns
WHERE table_schema = 'np_common' AND table_name = 'schema_versions'
  AND column_name IN ('applied_at', 'plugin', 'version');`)
	if err != nil {
		return fmt.Errorf("inspecting np_common.schema_versions: %w", err)
	}
	if !strings.Contains(cols, "plugin") || !strings.Contains(cols, "version") {
		return nil
	}
	appliedAt := "NOW()"
	if strings.Contains(cols, "applied_at") {
		appliedAt = "MIN(applied_at)"
	}
	// ORDER BY gives concurrent callers the same insert order, so two
	// installs racing through this copy cannot deadlock on the primary key.
	copySQL := fmt.Sprintf(`INSERT INTO np_common.plugin_schema_versions (plugin, version, applied_at)
SELECT plugin, version, %s FROM np_common.schema_versions
WHERE plugin IS NOT NULL AND version IS NOT NULL
GROUP BY plugin, version
ORDER BY plugin, version
ON CONFLICT (plugin, version) DO NOTHING;`, appliedAt)
	if err := execPSQL(ctx, cfg, copySQL); err != nil {
		return fmt.Errorf("carrying plugin rows into np_common.plugin_schema_versions: %w", err)
	}
	return nil
}
