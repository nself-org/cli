package commands

import (
	"github.com/spf13/cobra"
)

// ── migrate audit ────────────────────────────────────────────────────

var dbMigrateAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Audit migrations for idempotency, rollback files, and checksum drift",
	Long: `Audit all migration files for:
  - Idempotency: safe-to-rerun SQL patterns (IF NOT EXISTS, CREATE OR REPLACE)
  - Rollback coverage: presence of a corresponding down.sql
  - Checksum drift: on-disk file matches the checksum recorded on apply`,
	RunE: runDBMigrateAudit,
}

// ── migrate idempotent ───────────────────────────────────────────────

var dbMigrateIdempotentCmd = &cobra.Command{
	Use:   "idempotent <file>",
	Short: "Check and optionally rewrite a migration file to be idempotent",
	Long: `Analyzes a migration SQL file for non-idempotent patterns and suggests
(or generates) an idempotent version using IF NOT EXISTS / IF EXISTS clauses.`,
	Args: cobra.ExactArgs(1),
	RunE: runDBMigrateIdempotent,
}

// ── db drift ─────────────────────────────────────────────────────────

var dbDriftCmd = &cobra.Command{
	Use:     "drift",
	Aliases: []string{"schema-drift"},
	Short:   "Schema drift detection: scan np_* tables for Theme 25 column compliance",
	Long: `Detect schema drift across all np_* tables.

Theme 25 mandates 5 columns on every np_* table:
  id         UUID primary key
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
  user_id    UUID (nullable, owner reference)
  deleted_at TIMESTAMPTZ (soft-delete)

Subcommands:
  scan   Report missing columns per table
  fix    Generate migration SQL to add missing columns

Flags:
  --metadata  Detect Hasura METADATA drift instead (permissions, relationships,
              table tracking) against a live instance — see 'nself db drift --metadata'`,
	// Without NoArgs an unknown subcommand falls through to this RunE, prints
	// the help text and exits 0 — a silent success a caller can capture as
	// real output (see env.go: the golden path curled the help prose as a base
	// URL). Bare `nself db drift` still prints help, and the --metadata branch
	// takes no positional args (runDBDriftMetadata ignores them).
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		metadata, _ := cmd.Flags().GetBool("metadata")
		if metadata {
			return runDBDriftMetadata(cmd, args)
		}
		return cmd.Help()
	},
}

var dbDriftScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan np_* tables for missing Theme 25 columns",
	RunE:  runDBDriftScan,
}

var dbDriftFixCmd = &cobra.Command{
	Use:   "fix [schema table]",
	Short: "Generate migration SQL to fix Theme 25 column drift",
	Long: `Generate up.sql and down.sql migration content for tables with missing columns.

With no arguments, lists drifted tables. Use --all to generate fixes for all drifted tables.
With schema and table arguments, generates fixes for the specified table only.`,
	Args: cobra.MaximumNArgs(2),
	RunE: runDBDriftFix,
}

// ── run functions ────────────────────────────────────────────────────

func init() {
	dbMigrateCmd.AddCommand(dbMigrateAuditCmd)
	dbMigrateCmd.AddCommand(dbMigrateIdempotentCmd)

	dbDriftScanCmd.Flags().String("schema", "", "Filter to specific schema")
	dbDriftFixCmd.Flags().Bool("all", false, "Fix drift on all drifted tables")
	dbDriftCmd.Flags().Bool("metadata", false, "Detect Hasura metadata drift (permissions, relationships, table tracking) instead of column drift")
	dbDriftCmd.Flags().String("env", "", "Project directory to read hasura/metadata/** from (default: current directory)")

	dbDriftCmd.AddCommand(dbDriftScanCmd)
	dbDriftCmd.AddCommand(dbDriftFixCmd)
	dbCmd.AddCommand(dbDriftCmd)
}
