package commands

// Purpose: RunE implementations for "nself db drift scan" and "nself db
// drift fix" plus the small formatting helpers they use (auditBoolMark,
// truncateStr). Inputs are the cobra command/args; outputs are printed drift
// results or an error.
// Constraints: split out of db_audit.go (CLI-R12) as a pure move, no behavior change.

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/database"
	"github.com/spf13/cobra"
)

func runDBDriftScan(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	schemaFilter, _ := cmd.Flags().GetString("schema")

	results, err := database.ScanSchemaDrift(cmd.Context(), cfg)
	if err != nil {
		return fmt.Errorf("scan schema drift: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No np_* tables found.")
		return nil
	}

	if schemaFilter != "" {
		var filtered []database.SchemaDriftResult
		for _, r := range results {
			if r.TableSchema == schemaFilter {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	summary := database.SummarizeDrift(results)
	setData(cmd, newDriftScanData(results, summary))

	// Group by schema for display.
	schemaOrder := []string{}
	schemaMap := make(map[string][]database.SchemaDriftResult)
	for _, r := range results {
		if _, ok := schemaMap[r.TableSchema]; !ok {
			schemaOrder = append(schemaOrder, r.TableSchema)
		}
		schemaMap[r.TableSchema] = append(schemaMap[r.TableSchema], r)
	}

	for _, sname := range schemaOrder {
		tables := schemaMap[sname]
		drifted := 0
		for _, t := range tables {
			if len(t.MissingColumns) > 0 {
				drifted++
			}
		}

		fmt.Printf("\nSchema: %s (%d tables, %d drifted)\n", sname, len(tables), drifted)
		fmt.Printf("  %-30s  %-4s  %-10s  %-10s  %-7s  %-10s  SCORE\n",
			"TABLE", "ID", "CREATED_AT", "UPDATED_AT", "USER_ID", "DELETED_AT")
		fmt.Printf("  %s\n", strings.Repeat("-", 82))

		for _, r := range tables {
			presentSet := make(map[string]bool, len(r.PresentColumns))
			for _, c := range r.PresentColumns {
				presentSet[c] = true
			}

			colMark := func(name string) string {
				if presentSet[name] {
					return "ok"
				}
				return "x"
			}

			fmt.Printf("  %-30s  %-4s  %-10s  %-10s  %-7s  %-10s  %d\n",
				truncateStr(r.TableName, 30),
				colMark("id"),
				colMark("created_at"),
				colMark("updated_at"),
				colMark("user_id"),
				colMark("deleted_at"),
				r.DriftScore,
			)
		}
	}

	fmt.Printf("\nSummary: %d tables total, %d compliant, %d drifted, %d missing required columns, overall score: %d/100\n",
		summary.TotalTables,
		summary.CompliantTables,
		summary.DriftedTables,
		summary.MissingRequired,
		summary.OverallScore,
	)

	if summary.DriftedTables > 0 {
		fmt.Println("\nRun 'nself db drift fix' to generate migration SQL for drifted tables.")
	}
	return nil
}

func runDBDriftFix(cmd *cobra.Command, args []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	fixAll, _ := cmd.Flags().GetBool("all")

	results, err := database.ScanSchemaDrift(cmd.Context(), cfg)
	if err != nil {
		return fmt.Errorf("scan schema drift: %w", err)
	}

	var drifted []database.SchemaDriftResult
	for _, r := range results {
		if len(r.MissingColumns) > 0 {
			drifted = append(drifted, r)
		}
	}

	if len(drifted) == 0 {
		fmt.Println("No drift detected. All np_* tables are Theme 25 compliant.")
		return nil
	}

	// Filter by specific schema+table if args provided and not --all.
	if !fixAll && len(args) == 2 {
		schema := args[0]
		table := args[1]
		var single []database.SchemaDriftResult
		for _, r := range drifted {
			if r.TableSchema == schema && r.TableName == table {
				single = append(single, r)
			}
		}
		if len(single) == 0 {
			fmt.Printf("No drift found for %s.%s\n", schema, table)
			return nil
		}
		drifted = single
	} else if !fixAll && len(args) == 0 {
		// Show list without generating SQL.
		fmt.Println("Drifted tables (pass schema and table, or use --all):")
		for _, r := range drifted {
			cols := make([]string, 0, len(r.MissingColumns))
			for _, c := range r.MissingColumns {
				cols = append(cols, c.Name)
			}
			fmt.Printf("  %s.%s  missing: %s\n", r.TableSchema, r.TableName, strings.Join(cols, ", "))
		}
		return nil
	}

	for _, r := range drifted {
		upSQL, downSQL := database.GenerateDriftMigration(r)
		if upSQL == "" {
			continue
		}
		fmt.Printf("-- up.sql for %s.%s\n", r.TableSchema, r.TableName)
		fmt.Println(upSQL)
		fmt.Printf("\n-- down.sql for %s.%s\n", r.TableSchema, r.TableName)
		fmt.Println(downSQL)
		fmt.Println()
	}

	return nil
}

// ── helpers ──────────────────────────────────────────────────────────

func auditBoolMark(v bool) string {
	if v {
		return "ok"
	}
	return "x"
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "~"
}

// ── init ─────────────────────────────────────────────────────────────
