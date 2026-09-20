package commands

import (
	"github.com/spf13/cobra"
)

// ── db pitr ─────────────────────────────────────────────────────────

var dbPITRCmd = &cobra.Command{
	Use:   "pitr",
	Short: "Point-in-time recovery: status, enable, test, restore",
	Long: `Point-in-time recovery (PITR) operations for PostgreSQL WAL archiving.

Subcommands:
  status   Show current WAL archiving configuration
  enable   Write PITR configuration to .nself/pitr/
  test     Verify WAL archiving is working
  restore  Restore database to a specific point in time`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself db pitr` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var dbPITRStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show PITR/WAL archiving status",
	RunE:  runDBPITRStatus,
}

var dbPITREnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Write PITR configuration to .nself/pitr/",
	RunE:  runDBPITREnable,
}

var dbPITRTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Verify WAL archiving is working",
	RunE:  runDBPITRTest,
}

var dbPITRRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore database to a point in time (--target RFC3339)",
	RunE:  runDBPITRRestore,
}

// ── db backup-sync ───────────────────────────────────────────────────

var dbBackupSyncCmd = &cobra.Command{
	Use:   "backup-sync",
	Short: "Sync local backups to remote storage (cross-region)",
	RunE:  runDBBackupSync,
}

var dbBackupSyncStatusCmd = &cobra.Command{
	Use:   "backup-sync-status",
	Short: "Show cross-region backup sync status",
	RunE:  runDBBackupSyncStatus,
}

// ── db restore-drill ─────────────────────────────────────────────────

var dbRestoreDrillCmd = &cobra.Command{
	Use:   "restore-drill",
	Short: "Run a non-destructive restore drill",
	Long: `Run a non-destructive restore drill.

Creates a temporary database, restores the most recent backup into it,
verifies data integrity, then drops the temporary database. The production
database is never touched.`,
	RunE: runDBRestoreDrill,
}

var dbRestoreDrillListCmd = &cobra.Command{
	Use:   "restore-drill-list",
	Short: "List past restore drill results",
	RunE:  runDBRestoreDrillList,
}

// ── init ─────────────────────────────────────────────────────────────

func init() {
	// pitr subcommands
	dbPITRRestoreCmd.Flags().String("target", "", "Target time in RFC3339 format (e.g. 2024-01-15T14:30:00Z)")

	dbPITRCmd.AddCommand(dbPITRStatusCmd)
	dbPITRCmd.AddCommand(dbPITREnableCmd)
	dbPITRCmd.AddCommand(dbPITRTestCmd)
	dbPITRCmd.AddCommand(dbPITRRestoreCmd)

	dbCmd.AddCommand(dbPITRCmd)

	// backup-sync flags
	dbBackupSyncCmd.Flags().String("remote", "", "rclone remote path (e.g. s3:mybucket/backups)")
	dbBackupSyncCmd.Flags().Bool("encrypt", false, "Encrypt backup files with age before syncing")
	dbBackupSyncCmd.Flags().String("recipients", "", "age public key recipient(s) for encryption")

	dbCmd.AddCommand(dbBackupSyncCmd)
	dbCmd.AddCommand(dbBackupSyncStatusCmd)

	// restore-drill flags
	dbRestoreDrillCmd.Flags().String("file", "", "Backup file to use (default: most recent)")

	dbCmd.AddCommand(dbRestoreDrillCmd)
	dbCmd.AddCommand(dbRestoreDrillListCmd)
}

// ── run functions ─────────────────────────────────────────────────────
