package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/database"
	"github.com/spf13/cobra"
)

// ── nself db rls ─────────────────────────────────────────────────────

var dbRLSCmd = &cobra.Command{
	Use:   "rls",
	Short: "Row-Level Security management: audit, apply, rollback",
	Long: `Row-Level Security (RLS) management for nSelf tables.

Subcommands:
  audit        List all tables with their RLS and policy state
  apply        Enable RLS + default policies on tables missing them
  apply-table  Enable RLS + default policies on a specific schema.table
  rollback     Print rollback SQL that removes policies and disables RLS`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself db rls` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// ── nself db rls audit ───────────────────────────────────────────────

var dbRLSAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "List tables with their RLS and policy state",
	Long: `Query pg_catalog.pg_class and pg_policies to show the RLS state of every
table in schemas starting with "np_" or in schema "public".

Output columns: SCHEMA | TABLE | RLS | POLICIES`,
	RunE: runDBRLSAudit,
}

func runDBRLSAudit(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	tables, err := database.AuditRLS(cmd.Context(), cfg)
	if err != nil {
		return fmt.Errorf("db rls audit: %w", err)
	}

	if len(tables) == 0 {
		fmt.Println("No tables found in np_* or public schemas.")
		return nil
	}

	format, _ := cmd.Flags().GetString("format")
	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(tables)
	}

	// Table output.
	fmt.Printf("%-20s  %-35s  %-6s  %s\n", "SCHEMA", "TABLE", "RLS", "POLICIES")
	for _, t := range tables {
		rlsMark := "x"
		if t.RLSEnabled {
			rlsMark = "v"
		}
		fmt.Printf("%-20s  %-35s  %-6s  %d\n", t.TableSchema, t.TableName, rlsMark, t.PolicyCount)
	}
	return nil
}

// ── nself db rls apply ───────────────────────────────────────────────

var dbRLSApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Enable RLS and apply default policies on tables that are missing them",
	Long: `Queries all tables in np_* and public schemas, then enables RLS and applies
the four standard Hasura-compatible policies (select/insert/update/delete) on
any table that does not already have RLS enabled with at least one policy.

Flags:
  --dry-run   Print generated SQL without executing it
  --pattern   Filter tables by name glob (e.g. "np_claw_*")`,
	RunE: runDBRLSApply,
}

func runDBRLSApply(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	tables, err := database.AuditRLS(cmd.Context(), cfg)
	if err != nil {
		return fmt.Errorf("db rls apply: %w", err)
	}

	pattern, _ := cmd.Flags().GetString("pattern")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	// Filter to tables that need RLS applied.
	var targets []database.RLSTableInfo
	for _, t := range tables {
		if t.RLSEnabled && t.PolicyCount > 0 {
			continue
		}
		if pattern != "" && !matchesGlob(pattern, t.TableSchema+"_"+t.TableName) && !matchesGlob(pattern, t.TableName) {
			continue
		}
		targets = append(targets, t)
	}

	if len(targets) == 0 {
		fmt.Println("All tables already have RLS enabled.")
		return nil
	}

	if dryRun {
		fmt.Printf("-- dry-run: would apply RLS to %d table(s)\n", len(targets))
		for _, t := range targets {
			fmt.Printf("-- %s.%s (rls_enabled=%v, policies=%d)\n", t.TableSchema, t.TableName, t.RLSEnabled, t.PolicyCount)
			rlsCfg := database.TenantRLSConfig{
				Pattern: database.PatternUserOwned,
				Schema:  t.TableSchema,
				Table:   t.TableName,
			}
			sql, genErr := database.GenerateTenantRLSSQL(rlsCfg)
			if genErr != nil {
				fmt.Printf("-- error generating SQL for %s.%s: %v\n", t.TableSchema, t.TableName, genErr)
				continue
			}
			fmt.Println(sql)
		}
		return nil
	}

	applied, skipped, errs := database.ApplyRLSBatch(cmd.Context(), cfg, tables)
	fmt.Printf("RLS apply: %d applied, %d skipped", applied, skipped)
	if len(errs) > 0 {
		fmt.Printf(", %d error(s)\n", len(errs))
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  error: %v\n", e)
		}
		return fmt.Errorf("db rls apply: %d error(s) encountered", len(errs))
	}
	fmt.Println()
	return nil
}

// ── nself db rls apply-table ─────────────────────────────────────────

var dbRLSApplyTableCmd = &cobra.Command{
	Use:   "apply-table <schema> <table>",
	Short: "Enable RLS and apply default policies on a specific table",
	Long: `Enables row-level security and creates the four standard Hasura-compatible
policies on the specified schema.table. The table must exist in the database.

Example:
  nself db rls apply-table np_claw claw_messages`,
	Args: cobra.ExactArgs(2),
	RunE: runDBRLSApplyTable,
}

func runDBRLSApplyTable(cmd *cobra.Command, args []string) error {
	schema := args[0]
	table := args[1]

	if err := validateIdentifier(schema); err != nil {
		return fmt.Errorf("invalid schema name: %w", err)
	}
	if err := validateIdentifier(table); err != nil {
		return fmt.Errorf("invalid table name: %w", err)
	}

	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	if err := database.EnableRLSOnTable(cmd.Context(), cfg, schema, table); err != nil {
		return fmt.Errorf("db rls apply-table: %w", err)
	}
	if err := database.ApplyDefaultRLSPolicies(cmd.Context(), cfg, schema, table); err != nil {
		return fmt.Errorf("db rls apply-table: %w", err)
	}

	fmt.Printf("RLS enabled and policies applied on %s.%s.\n", schema, table)
	return nil
}

// ── nself db rls rollback ────────────────────────────────────────────

var dbRLSRollbackCmd = &cobra.Command{
	Use:   "rollback <schema> <table>",
	Short: "Print rollback SQL that removes RLS policies and disables RLS",
	Long: `Generates and prints the SQL needed to remove all four standard RLS policies
and disable row-level security on the specified table. The SQL is printed to
stdout so you can review and apply it manually.

Example:
  nself db rls rollback np_claw claw_messages | psql ...`,
	Args: cobra.ExactArgs(2),
	RunE: runDBRLSRollback,
}

func runDBRLSRollback(_ *cobra.Command, args []string) error {
	schema := args[0]
	table := args[1]

	if err := validateIdentifier(schema); err != nil {
		return fmt.Errorf("invalid schema name: %w", err)
	}
	if err := validateIdentifier(table); err != nil {
		return fmt.Errorf("invalid table name: %w", err)
	}

	sql := database.GenerateRollbackSQL(schema, table)
	fmt.Print(sql)
	return nil
}

// ── init ─────────────────────────────────────────────────────────────

func init() {
	dbCmd.AddCommand(dbRLSCmd)
	dbRLSCmd.AddCommand(dbRLSAuditCmd, dbRLSApplyCmd, dbRLSApplyTableCmd, dbRLSRollbackCmd)

	dbRLSApplyCmd.Flags().Bool("dry-run", false, "print SQL without executing")
	dbRLSApplyCmd.Flags().String("pattern", "", "table name glob pattern (e.g. np_claw_*)")
	dbRLSAuditCmd.Flags().String("format", "table", "output format: table or json")
}

// ── helpers ───────────────────────────────────────────────────────────

// validateIdentifier rejects schema and table names that contain characters
// not valid in a PostgreSQL unquoted identifier. This prevents injection via
// apply-table arguments even though the names are also double-quoted.
func validateIdentifier(name string) error {
	if name == "" {
		return fmt.Errorf("name must not be empty")
	}
	for _, r := range name {
		if !isIdentChar(r) {
			return fmt.Errorf("character %q is not allowed in an identifier", r)
		}
	}
	return nil
}

func isIdentChar(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_'
}

// matchesGlob performs a simple prefix/suffix/contains glob match supporting
// a single '*' wildcard. It does not support full filepath glob patterns.
func matchesGlob(pattern, name string) bool {
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == name
	}
	parts := strings.SplitN(pattern, "*", 2)
	prefix, suffix := parts[0], parts[1]
	if len(prefix) > 0 && !strings.HasPrefix(name, prefix) {
		return false
	}
	if len(suffix) > 0 && !strings.HasSuffix(name, suffix) {
		return false
	}
	return true
}
