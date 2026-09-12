// Package database — restore_drill_verify.go: post-restore verification
// queries for RestoreDrill (VerifyRestoredDatabase, verifyCriticalTables).
// Split out of restore_drill.go (file-size ratchet, internal/repoqa) — this
// is the self-contained "what does the restored DB actually contain" half of
// the drill, following the same drill_*.go concern-split convention already
// used for drill_critical_tables.go. No behavior change.
package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// VerifyRestoredDatabase connects to a restored database and runs basic
// integrity checks: counts rows in key tables, verifies pg_catalog consistency.
func VerifyRestoredDatabase(ctx context.Context, cfg *config.Config, drillDB string) (tables int, rows int64, err error) {
	// Count user tables.
	tableCountSQL := `SELECT count(*) FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog','information_schema')`
	out, err := querySQL(ctx, cfg, drillDB, tableCountSQL)
	if err != nil {
		return 0, 0, fmt.Errorf("count tables in %s: %w", drillDB, err)
	}
	out = strings.TrimSpace(out)
	tableCount := 0
	if out != "" {
		if _, scanErr := fmt.Sscanf(out, "%d", &tableCount); scanErr != nil {
			return 0, 0, fmt.Errorf("parse table count %q: %w", out, scanErr)
		}
	}

	// Exact total row count across ALL user tables, in one round trip.
	//
	// The prior approach sampled up to 10 tables (LIMIT 10, no ORDER BY) and
	// summed their row counts. On a schema with many tables that sample could
	// land entirely on empty ones and report zero rows for a perfectly good
	// restore — a false fail. It could just as easily miss the tables that
	// actually hold data and report zero for the opposite reason: a false
	// pass. Neither is acceptable once the caller (drill.go) starts asserting
	// on this number.
	//
	// query_to_xml lets Postgres build and run one dynamic COUNT(*) per table
	// server-side and return all the results in a single query. format('%I.%I', ...)
	// is Postgres's own identifier quoting (SEC-SQL-01: table_schema/table_name
	// here are DB-sourced and must never be interpolated by the Go side without
	// it — %I does that quoting inside the server, so no client-side
	// SanitizeIdentifier call is needed for this query).
	rowTotalSQL := `SELECT COALESCE(SUM((xpath('/row/c/text()', ` +
		`query_to_xml(format('SELECT count(*) AS c FROM %I.%I', table_schema, table_name), false, true, '')` +
		`))[1]::text::bigint), 0) FROM information_schema.tables ` +
		`WHERE table_schema NOT IN ('pg_catalog','information_schema')`
	rowOut, err := querySQL(ctx, cfg, drillDB, rowTotalSQL)
	if err != nil {
		return tableCount, 0, fmt.Errorf("count total rows in %s: %w", drillDB, err)
	}
	rowOut = strings.TrimSpace(rowOut)
	var totalRows int64
	if rowOut != "" {
		if _, scanErr := fmt.Sscanf(rowOut, "%d", &totalRows); scanErr != nil {
			return tableCount, 0, fmt.Errorf("parse row total %q: %w", rowOut, scanErr)
		}
	}

	// Verify pg_catalog is accessible by probing a relation size.
	catalogSQL := `SELECT pg_catalog.pg_relation_size(c.oid) FROM pg_catalog.pg_class c WHERE c.relname = 'pg_class' LIMIT 1`
	if _, catErr := querySQL(ctx, cfg, drillDB, catalogSQL); catErr != nil {
		return tableCount, totalRows, fmt.Errorf("pg_catalog probe failed in %s: %w", drillDB, catErr)
	}

	return tableCount, totalRows, nil
}

// verifyCriticalTables reports which entries of ResolveCriticalTables(cfg)
// (drill.go — DefaultCriticalTables unless the project sets
// BACKUP_CRITICAL_TABLES) are missing by name, in any schema, from drillDB.
// The resolved names are project config, not DB-sourced input, so they are
// safe to place directly into a SQL literal list here (properly
// single-quote-escaped below) — SEC-SQL-01's "never interpolate DB-sourced
// identifiers without quoting" concerns table_schema/table_name values read
// back FROM the database (handled via format('%I.%I') in
// VerifyRestoredDatabase above), not our own resolved string-literal values.
func verifyCriticalTables(ctx context.Context, cfg *config.Config, drillDB string) ([]string, error) {
	criticalTables := ResolveCriticalTables(cfg)
	literals := make([]string, len(criticalTables))
	for i, name := range criticalTables {
		literals[i] = "'" + strings.ReplaceAll(name, "'", "''") + "'"
	}
	sqlText := fmt.Sprintf(
		`SELECT DISTINCT table_name FROM information_schema.tables WHERE table_name = ANY(ARRAY[%s])`,
		strings.Join(literals, ","),
	)
	out, err := querySQL(ctx, cfg, drillDB, sqlText)
	if err != nil {
		return nil, fmt.Errorf("query critical tables in %s: %w", drillDB, err)
	}
	present := make(map[string]bool, len(criticalTables))
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			present[line] = true
		}
	}
	var missing []string
	for _, name := range criticalTables {
		if !present[name] {
			missing = append(missing, name)
		}
	}
	return missing, nil
}
