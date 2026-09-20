package commands

import (
	"fmt"

	"github.com/nself-org/cli/internal/database"
	"github.com/spf13/cobra"
)

// ── db soft-delete ───────────────────────────────────────────────────

var dbSoftDeleteCmd = &cobra.Command{
	Use:   "soft-delete",
	Short: "Manage soft-delete (deleted_at) patterns across tables",
	Long: `Audit and apply soft-delete patterns across database tables.

Subcommands:
  audit     Report soft-delete status for all tables
  apply     Add soft-delete support to a specific table
  generate  Print migration SQL to stdout without executing`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself db soft-delete` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var dbSoftDeleteAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Audit tables for soft-delete status",
	RunE:  runDBSoftDeleteAudit,
}

var dbSoftDeleteApplyCmd = &cobra.Command{
	Use:   "apply <schema> <table>",
	Short: "Add deleted_at column, index, and active view to a table",
	Args:  cobra.ExactArgs(2),
	RunE:  runDBSoftDeleteApply,
}

var dbSoftDeleteGenerateCmd = &cobra.Command{
	Use:   "generate <schema> <table>",
	Short: "Print soft-delete migration SQL to stdout",
	Args:  cobra.ExactArgs(2),
	RunE:  runDBSoftDeleteGenerate,
}

// ── db fk-index ──────────────────────────────────────────────────────

var dbFKIndexCmd = &cobra.Command{
	Use:   "fk-index",
	Short: "Audit and create indexes for foreign key columns",
	Long: `Find foreign key columns that lack a supporting index and optionally create them.

Subcommands:
  audit   Report FK columns with and without indexes
  apply   Create missing FK indexes (use --dry-run to preview)`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself db fk-index` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var dbFKIndexAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Find FK columns without supporting indexes",
	RunE:  runDBFKIndexAudit,
}

var dbFKIndexApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Create missing FK indexes",
	RunE:  runDBFKIndexApply,
}

// ── init ─────────────────────────────────────────────────────────────

func init() {
	dbCmd.AddCommand(dbSoftDeleteCmd)
	dbSoftDeleteCmd.AddCommand(dbSoftDeleteAuditCmd, dbSoftDeleteApplyCmd, dbSoftDeleteGenerateCmd)

	dbCmd.AddCommand(dbFKIndexCmd)
	dbFKIndexCmd.AddCommand(dbFKIndexAuditCmd, dbFKIndexApplyCmd)
	dbFKIndexApplyCmd.Flags().Bool("dry-run", false, "print SQL without executing")
}

// ── run functions ─────────────────────────────────────────────────────

func runDBSoftDeleteAudit(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	findings, err := database.AuditSoftDelete(cmd.Context(), cfg)
	if err != nil {
		return fmt.Errorf("soft-delete audit: %w", err)
	}
	if len(findings) == 0 {
		fmt.Println("No tables found.")
		return nil
	}

	checkmark := func(b bool) string {
		if b {
			return "\u2713"
		}
		return "\u2717"
	}

	fmt.Printf("%-12s %-30s %-16s %-7s %-5s\n", "SCHEMA", "TABLE", "HAS_DELETED_AT", "INDEX", "VIEW")
	for _, f := range findings {
		fmt.Printf("%-12s %-30s %-16s %-7s %-5s\n",
			f.TableSchema,
			f.TableName,
			checkmark(f.HasDeletedAt),
			checkmark(f.HasIndex),
			checkmark(f.HasView),
		)
	}
	return nil
}

func runDBSoftDeleteApply(cmd *cobra.Command, args []string) error {
	schema := args[0]
	table := args[1]

	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	if err := database.ApplySoftDelete(cmd.Context(), cfg, schema, table); err != nil {
		return err
	}
	fmt.Printf("Soft-delete applied to %s.%s.\n", schema, table)
	return nil
}

func runDBSoftDeleteGenerate(_ *cobra.Command, args []string) error {
	schema := args[0]
	table := args[1]

	upSQL, downSQL := database.GenerateSoftDeleteMigration(schema, table)
	fmt.Printf("-- up.sql\n%s\n-- down.sql\n%s", upSQL, downSQL)
	return nil
}

func runDBFKIndexAudit(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	findings, err := database.AuditFKIndexes(cmd.Context(), cfg)
	if err != nil {
		return fmt.Errorf("fk-index audit: %w", err)
	}
	if len(findings) == 0 {
		fmt.Println("No foreign key columns found.")
		return nil
	}

	checkmark := func(b bool) string {
		if b {
			return "\u2713"
		}
		return "\u2717"
	}

	fmt.Printf("%-12s %-20s %-15s %-25s %-10s\n", "SCHEMA", "TABLE", "COLUMN", "FOREIGN TABLE", "HAS_INDEX")
	for _, f := range findings {
		foreignRef := fmt.Sprintf("%s.%s", f.ForeignSchema, f.ForeignTable)
		fmt.Printf("%-12s %-20s %-15s %-25s %-10s\n",
			f.TableSchema,
			f.TableName,
			f.ColumnName,
			foreignRef,
			checkmark(f.HasIndex),
		)
	}
	return nil
}

func runDBFKIndexApply(cmd *cobra.Command, _ []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	findings, err := database.AuditFKIndexes(cmd.Context(), cfg)
	if err != nil {
		return fmt.Errorf("fk-index audit: %w", err)
	}

	var missing []database.FKIndexInfo
	for _, f := range findings {
		if !f.HasIndex {
			missing = append(missing, f)
		}
	}

	if len(missing) == 0 {
		fmt.Println("All FK columns already have indexes.")
		return nil
	}

	if dryRun {
		fmt.Println("-- dry-run: SQL that would be executed")
		for _, f := range missing {
			fmt.Println(database.GenerateFKIndexSQL(f))
		}
		return nil
	}

	created, applyErrs := database.ApplyFKIndexes(cmd.Context(), cfg, findings)
	fmt.Printf("Created %d FK index(es).\n", created)
	if len(applyErrs) > 0 {
		for _, e := range applyErrs {
			fmt.Printf("error: %v\n", e)
		}
		return fmt.Errorf("%d FK index(es) failed to create", len(applyErrs))
	}
	return nil
}
