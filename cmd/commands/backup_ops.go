package commands

import (
	"github.com/spf13/cobra"
)

// ── Parent command ──────────────────────────────────────────────────

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup operations: create, list, restore, verify, prune, config, status, init-key",
	Long: `Backup operations for nSelf projects.

Subcommands:
  create     Create a new backup (full, wal, metadata, minio, all)
  list       List available backups
  restore    Restore from a backup
  verify     Verify backup integrity
  prune      Remove old backups by retention policy
  config     View/set backup configuration
  status     Show backup subsystem status
  init-key   Generate age encryption keypair`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself backup` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// ── backup create ──────────────────────────────────────────────────

func init() {
	// backup create flags
	backupCreateCmd.Flags().String("type", "full", "Backup type: full, wal, metadata, minio, all")
	backupCreateCmd.Flags().String("remote", "", "Remote name override")
	backupCreateCmd.Flags().Bool("encrypt", false, "Force encryption on")
	backupCreateCmd.Flags().Bool("no-encrypt", false, "Force encryption off")
	backupCreateCmd.Flags().String("tag", "", "Human label for this backup")
	backupCreateCmd.Flags().Bool("dry-run", false, "Preview only")

	// backup list flags
	backupListCmd.Flags().String("remote", "", "Filter by remote name")
	backupListCmd.Flags().String("env", "", "Filter by environment")
	backupListCmd.Flags().String("since", "", "Show backups newer than this duration (e.g. 24h, 7d)")
	backupListCmd.Flags().String("format", "table", "Output format: table or json")

	// backup restore flags
	backupRestoreCmd.Flags().String("to", "", "Restore to alternate directory")
	backupRestoreCmd.Flags().String("only", "", "Restore subset: pg,minio,metadata (comma-separated)")
	backupRestoreCmd.Flags().String("point-in-time", "", "ISO8601 timestamp for PITR")
	backupRestoreCmd.Flags().String("decrypt-key", "", "Path to age identity file")
	backupRestoreCmd.Flags().Bool("yes", false, "Skip confirmation")

	// backup verify flags
	backupVerifyCmd.Flags().Bool("restore-test", false, "Spin up test container and restore")
	backupVerifyCmd.Flags().Bool("cleanup", true, "Remove test container after verify")
	backupVerifyCmd.Flags().Bool("keep", false, "Keep test container for inspection")

	// backup prune flags
	backupPruneCmd.Flags().Bool("dry-run", false, "Preview only")
	backupPruneCmd.Flags().Int("keep-daily", 7, "Keep last N daily backups")
	backupPruneCmd.Flags().Int("keep-weekly", 4, "Keep last N weekly backups")
	backupPruneCmd.Flags().Int("keep-monthly", 12, "Keep last N monthly backups")
	backupPruneCmd.Flags().String("format", "", "Output format: json")

	// backup config flags
	backupConfigCmd.Flags().String("format", "", "Output format: json")
	backupConfigCmd.Flags().Bool("install-cron", false, "Install systemd timers for backup/wal/prune/verify")
	backupConfigCmd.Flags().String("full-at", "03:00", "Full backup time UTC (HH:MM) when --install-cron")
	backupConfigCmd.Flags().String("wal-every", "15m", "WAL checkpoint interval when --install-cron")
	backupConfigCmd.Flags().String("prune-at", "04:00", "Prune time UTC (HH:MM) when --install-cron")
	backupConfigCmd.Flags().String("verify-on", "Sun", "Weekly restore-test day when --install-cron")
	backupConfigCmd.Flags().String("verify-at", "05:00", "Weekly restore-test time UTC (HH:MM) when --install-cron")
	backupConfigCmd.Flags().String("remote", "", "Override configured remote when --install-cron")
	backupConfigCmd.Flags().String("unit-dir", "/etc/systemd/system", "Systemd unit directory when --install-cron")
	backupConfigCmd.Flags().Bool("dry-run", false, "Print unit files without writing when --install-cron")

	// backup status flags
	backupStatusCmd.Flags().String("format", "", "Output format: json")

	// backup stream flags
	backupStreamCmd.Flags().String("to", "", "Destination URL (rclone remote path, e.g. s3:bucket/prefix)")
	backupStreamCmd.Flags().StringArray("recipient", nil, "Encryption recipient: age key, SSH key, or github:<username> (repeatable)")
	backupStreamCmd.Flags().Bool("dry-run", false, "Preview without running")
	backupStreamCmd.Flags().Bool("no-encrypt", false,
		"Stream in the clear when no recipient is configured. Without this, a missing recipient is an error rather than a silent plaintext upload.")

	// backup resume flags (no extra flags beyond the positional backup-id)

	// backup schedule flags
	backupScheduleCmd.Flags().String("cron", "", "Cron expression (e.g. '0 2 * * *')")
	backupScheduleCmd.Flags().String("to", "", "Destination URL (rclone remote path)")
	backupScheduleCmd.Flags().String("recipient", "", "Default encryption recipient")
	backupScheduleCmd.Flags().String("unit-dir", "/etc/systemd/system", "Systemd unit directory")
	backupScheduleCmd.Flags().Bool("dry-run", false, "Print unit files without writing")

	// backup restore-remote flags
	backupRestoreRemoteCmd.Flags().String("from", "", "Source URL (rclone remote path)")
	backupRestoreRemoteCmd.Flags().String("key", "", "Path to age identity file for decryption")
	backupRestoreRemoteCmd.Flags().Bool("yes", false, "Skip confirmation on production")

	// Wire subcommands
	backupCmd.AddCommand(backupCreateCmd)
	backupCmd.AddCommand(backupListCmd)
	backupCmd.AddCommand(backupRestoreCmd)
	backupCmd.AddCommand(backupVerifyCmd)
	backupCmd.AddCommand(backupPruneCmd)
	backupCmd.AddCommand(backupConfigCmd)
	backupCmd.AddCommand(backupStatusCmd)
	backupCmd.AddCommand(backupInitKeyCmd)
	backupCmd.AddCommand(backupStreamCmd)
	backupCmd.AddCommand(backupResumeCmd)
	backupCmd.AddCommand(backupScheduleCmd)
	backupCmd.AddCommand(backupRestoreRemoteCmd)

	RootCmd.AddCommand(backupCmd)
}
