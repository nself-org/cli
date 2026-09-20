package commands

// Purpose: the remaining "nself db ..." cobra command declarations (reset,
// backup-list, checksum verify/reset, seed run/list/verify/graph, hasura
// subcommands, lint) plus the package init() that registers every db
// subcommand. Inputs/outputs are the cobra command tree itself.
// Constraints: split out of db.go (CLI-R12) as a pure move, no behavior change.

import (
	"github.com/spf13/cobra"
)

var dbResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Drop and recreate database (DESTRUCTIVE)",
	RunE:  runDBReset,
}

// ── backup list ─────────────────────────────────────────────────────

var dbBackupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available backups with size and date",
	RunE:  runDBBackupList,
}

// ── verify-checksums ───────────────────────────────────────────────

var dbVerifyChecksumsCmd = &cobra.Command{
	Use:   "verify-checksums",
	Short: "Verify migration file checksums against stored values",
	RunE:  runDBVerifyChecksums,
}

// ── reset-checksum ─────────────────────────────────────────────────

var dbResetChecksumCmd = &cobra.Command{
	Use:   "reset-checksum <id>",
	Short: "Reset stored checksum for a migration (dangerous)",
	Args:  cobra.ExactArgs(1),
	RunE:  runDBResetChecksum,
}

// ── seed subcommands ───────────────────────────────────────────────

var dbSeedRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Execute seeds for current environment",
	RunE:  runDBSeedRun,
}

var dbSeedListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available seeds and fixtures",
	RunE:  runDBSeedList,
}

var dbSeedVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify a fixture is deterministic (dry-run replay check)",
	RunE:  runDBSeedVerify,
}

var dbSeedGraphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Show seed dependency graph",
	RunE:  runDBSeedGraph,
}

// ── hasura ──────────────────────────────────────────────────────────

var dbHasuraCmd = &cobra.Command{
	Use:   "hasura",
	Short: "Hasura metadata operations",
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself db hasura` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var dbHasuraConsoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Open Hasura Console",
	RunE:  runDBHasuraConsole,
}

var dbHasuraMetadataCmd = &cobra.Command{
	Use:   "metadata",
	Short: "Manage Hasura metadata",
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself db hasura metadata` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var dbHasuraMetadataApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply Hasura metadata",
	Long: `Apply Hasura metadata (tables, permissions, relationships, actions) to the
project's Hasura instance.

Pass --env staging|prod to apply against a deployed target's Hasura instead
of the local one: the command re-invokes 'nself db hasura metadata apply' on
the remote host over SSH, so the remote box always uses its own local
project/Hasura resolution. Note: this requires the remote host's nself CLI
version to support this subcommand — an older remote CLI returns a clear
version-drift error rather than a raw SSH failure.`,
	RunE: runDBHasuraMetadataApply,
}

var dbHasuraMetadataExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export Hasura metadata to git-friendly sorted YAML",
	RunE:  runDBHasuraMetadataExport,
}

var dbHasuraMetadataReloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload metadata cache",
	RunE:  runDBHasuraMetadataReload,
}

var dbHasuraDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Compare live Hasura metadata against on-disk files",
	RunE:  runDBHasuraDiff,
}

var dbHasuraValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate Hasura metadata consistency and permission coverage",
	RunE:  runDBHasuraValidate,
}

// ── lint ────────────────────────────────────────────────────────────

var dbLintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Check RLS policies on tenant-scoped tables",
	Long: `Audit Row-Level Security policies across all user-data tables.

By default, checks tables with tenant_id columns. With --rls, performs an
exhaustive audit of every np_* table and any table with user_id or tenant_id.

Flags:
  --rls       Exhaustive RLS audit (all np_* and user-data tables)
  --metric    Emit Prometheus-compatible metric line (nself_rls_disabled_tables)
  --matrix    Include table x role coverage matrix in output
  --format    Output format: table (default) or json
  --remediate Print remediation SQL for failing tables`,
	RunE: runDBLint,
}

func init() {
	// --force / --yes flags on reset (--force kept for backward compat)
	dbResetCmd.Flags().BoolVarP(&dbResetForce, "force", "f", false, "Skip confirmation prompt (for CI/automation)")
	dbResetCmd.Flags().Bool("yes", false, "Skip confirmation prompt (for CI/automation)")

	// --yes flag on drop and restore
	dbDropCmd.Flags().Bool("yes", false, "Skip confirmation prompt (for CI/automation)")
	dbRestoreCmd.Flags().Bool("overwrite", false, "Allow overwriting existing data")
	dbRestoreCmd.Flags().Bool("yes", false, "Skip confirmation prompt in production (for planned maintenance)")

	// --format flag on backup list
	dbBackupListCmd.Flags().String("format", "", "Output format: table (default) or json")

	// --plugin flag on migrate and its subcommands
	dbMigrateCmd.PersistentFlags().String("plugin", "", "Migrate specific plugin schema")

	// --dry-run flag on migrate up
	dbMigrateUpCmd.Flags().Bool("dry-run", false, "List pending migrations without applying them")
	// --migration-dir flag on migrate up (G-008)
	dbMigrateUpCmd.Flags().String("migration-dir", "", "Apply all .sql files in this directory in lexicographic order (skips already-applied)")
	// --migration-dir flag on migrate status (G-008): repos with non-standard
	// layouts (e.g. ntask postgres/migrations) otherwise report "No migrations found"
	dbMigrateStatusCmd.Flags().String("migration-dir", "", "Report status for migrations in this directory instead of the auto-detected one")
	// --detect flag on migrate status (cli#386): classify each pending
	// migration against the live schema instead of only checking the ledger.
	dbMigrateStatusCmd.Flags().Bool("detect", false, "Classify pending migrations against the live schema (BASELINE/APPLY/CONFLICT)")
	// --env/--server: remote targeting (gap #9) — default local, opt-in remote.
	addDBRemoteFlags(dbMigrateUpCmd)
	addDBRemoteFlags(dbMigrateStatusCmd)
	addDBRemoteFlags(dbHasuraMetadataApplyCmd)

	// --file flag on migrate apply (G-008)
	dbMigrateApplyCmd.Flags().String("file", "", "Path to the SQL migration file to apply")
	_ = dbMigrateApplyCmd.MarkFlagRequired("file")

	// Wire migrate subcommands
	dbMigrateCmd.AddCommand(dbMigrateUpCmd)
	dbMigrateCmd.AddCommand(dbMigrateDownCmd)
	dbMigrateCmd.AddCommand(dbMigrateStatusCmd)
	dbMigrateCmd.AddCommand(dbMigrateCreateCmd)
	dbMigrateCmd.AddCommand(dbMigrateApplyCmd)

	// Wire hasura metadata subcommands
	dbHasuraMetadataCmd.AddCommand(dbHasuraMetadataApplyCmd)
	dbHasuraMetadataCmd.AddCommand(dbHasuraMetadataExportCmd)
	dbHasuraMetadataCmd.AddCommand(dbHasuraMetadataReloadCmd)

	// Wire hasura subcommands
	dbHasuraCmd.AddCommand(dbHasuraConsoleCmd)
	dbHasuraCmd.AddCommand(dbHasuraMetadataCmd)
	dbHasuraCmd.AddCommand(dbHasuraDiffCmd)
	dbHasuraCmd.AddCommand(dbHasuraValidateCmd)

	// Wire backup subcommands
	dbBackupCmd.AddCommand(dbBackupListCmd)

	// db lint flags
	dbLintCmd.Flags().String("format", "table", "Output format: table or json")
	dbLintCmd.Flags().Bool("rls", false, "Exhaustive RLS audit (all np_* and user-data tables)")
	dbLintCmd.Flags().Bool("metric", false, "Emit Prometheus metric line (nself_rls_disabled_tables)")
	dbLintCmd.Flags().Bool("matrix", false, "Include table x role coverage matrix")
	dbLintCmd.Flags().Bool("remediate", false, "Print remediation SQL for failing tables")

	// verify-checksums and reset-checksum
	dbResetChecksumCmd.Flags().Bool("i-know-what-im-doing", false, "Required safety flag")

	// seed subcommands
	dbSeedRunCmd.Flags().String("env", "", "Target environment (default: current)")
	dbSeedRunCmd.Flags().String("fixture", "", "Run a specific fixture (e.g. demo, load-test)")
	dbSeedRunCmd.Flags().Bool("reset", false, "Truncate seeded tables before running (destructive)")
	dbSeedCmd.AddCommand(dbSeedRunCmd)
	dbSeedCmd.AddCommand(dbSeedListCmd)
	dbSeedCmd.AddCommand(dbSeedVerifyCmd)
	dbSeedCmd.AddCommand(dbSeedGraphCmd)
	dbSeedVerifyCmd.Flags().String("fixture", "", "Fixture to verify")

	// Wire top-level db subcommands
	dbCmd.AddCommand(dbLintCmd)
	dbCmd.AddCommand(dbMigrateCmd)
	dbCmd.AddCommand(dbSeedCmd)
	dbCmd.AddCommand(dbVerifyChecksumsCmd)
	dbCmd.AddCommand(dbResetChecksumCmd)
	dbCmd.AddCommand(dbBackupCmd)
	dbCmd.AddCommand(dbRestoreCmd)
	dbCmd.AddCommand(dbShellCmd)
	dbCmd.AddCommand(dbListCmd)
	dbCmd.AddCommand(dbDropCmd)
	dbCmd.AddCommand(dbResetCmd)
	dbCmd.AddCommand(dbHasuraCmd)

	RootCmd.AddCommand(dbCmd)
}

// ── helpers ─────────────────────────────────────────────────────────
