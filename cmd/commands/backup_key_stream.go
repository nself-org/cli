package commands

// Purpose: the "nself backup init-key" and "nself backup stream" subcommands
// and their RunE. Inputs are the cobra command/args; outputs are an
// initialized encryption key or a streamed backup, or an error.
// Constraints: split out of backup_ops.go (CLI-R12) as a pure move, no behavior change.

import (
	"fmt"

	"github.com/nself-org/cli/internal/backup"
	"github.com/spf13/cobra"
)

var backupInitKeyCmd = &cobra.Command{
	Use:   "init-key",
	Short: "Generate age encryption keypair for backups",
	RunE:  runBackupInitKey,
}

func runBackupInitKey(_ *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	pubKey, err := backup.InitKey(cfg)
	if err != nil {
		return fmt.Errorf("init-key: %w", err)
	}

	if pubKey != "" {
		fmt.Printf("Age keypair generated.\nPublic key: %s\n", pubKey)
		fmt.Println("Add to your .env: BACKUP_AGE_RECIPIENTS=" + pubKey)
	}
	return nil
}

// ── backup stream ──────────────────────────────────────────────────

var backupStreamCmd = &cobra.Command{
	Use:   "stream",
	Short: "Stream an encrypted backup directly to a remote destination",
	Long: `Stream a live backup directly to S3, R2, B2, GCS, or Azure Blob Storage.

The pipeline runs three concurrent stages with no temp files:

  pg_dump (streaming) | age (encrypt) | rclone rcat (multipart upload)

Encryption recipients may be age public keys, SSH public keys, or
GitHub user keys (e.g. github:username).

Examples:

  nself backup stream --to s3:bucket/path --recipient age1xxxxx
  nself backup stream --to r2:bucket/path --recipient github:username
  nself backup stream --to b2:bucket/path   # uses NSELF_BACKUP_RECIPIENT env
  nself backup stream --to r2:bucket/path --recipient age1xxxxx --heartbeat-to r2hb:hb

With --heartbeat-to (or NSELF_BACKUP_HEARTBEAT_REMOTE), a successful upload is
followed by a small <project>/backup.json object on that remote. A failed
backup never writes one. A heartbeat failure is a warning unless
--heartbeat-required is set.`,
	RunE: runBackupStream,
}

func init() {
	// Flags added with P7-PROD-71.
	backupStreamCmd.Flags().String("heartbeat-to", "", "rclone remote that receives <project>/backup.json after a successful upload (default: NSELF_BACKUP_HEARTBEAT_REMOTE)")
	backupStreamCmd.Flags().Bool("heartbeat-required", false, "Exit non-zero when the heartbeat cannot be written (the backup itself is kept)")
}

func runBackupStream(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	to, _ := cmd.Flags().GetString("to")
	recipients, _ := cmd.Flags().GetStringArray("recipient")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	noEncrypt, _ := cmd.Flags().GetBool("no-encrypt")
	heartbeatTo, _ := cmd.Flags().GetString("heartbeat-to")
	heartbeatRequired, _ := cmd.Flags().GetBool("heartbeat-required")

	result, err := backup.Stream(cmd.Context(), cfg, backup.StreamOptions{
		To:                to,
		Recipients:        recipients,
		DryRun:            dryRun,
		AllowUnencrypted:  noEncrypt,
		HeartbeatTo:       heartbeatTo,
		HeartbeatRequired: heartbeatRequired,
	})
	// A heartbeat failure under --heartbeat-required comes back with a
	// successful result: report the backup first, then fail.
	if err != nil && result == nil {
		return fmt.Errorf("backup stream: %w", err)
	}

	if dryRun {
		fmt.Printf("(dry-run) Would stream backup to: %s (encrypted: %v)\n",
			result.Destination, result.Encrypted)
		return nil
	}

	fmt.Printf("Streaming backup complete.\n")
	fmt.Printf("  Destination: %s\n", result.Destination)
	fmt.Printf("  Backup ID:   %s\n", result.BackupID)
	fmt.Printf("  Encrypted:   %v\n", result.Encrypted)
	fmt.Printf("  Duration:    %s\n", result.Duration)
	if err != nil {
		return fmt.Errorf("backup stream: the backup was uploaded but %w", err)
	}
	return nil
}

// ── backup resume ──────────────────────────────────────────────────
