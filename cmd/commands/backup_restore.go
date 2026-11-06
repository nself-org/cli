package commands

// Purpose: the "nself backup restore" and "nself backup restore-remote"
// subcommands and their RunE. Inputs are the cobra command/args; outputs are
// a restored database or an error.
// Constraints: both replace the database and share consent.Confirm (C2-01).

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nself-org/cli/internal/backup"
	"github.com/nself-org/cli/internal/backup/consent"
	"github.com/nself-org/cli/internal/config"
	"github.com/spf13/cobra"
)

var backupRestoreCmd = &cobra.Command{
	Use:   "restore <backup-id|latest>",
	Short: "Restore from a backup",
	Args:  cobra.ExactArgs(1),
	RunE:  dataEnv(runBackupRestore),
}

func runBackupRestore(cmd *cobra.Command, args []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	toDir, _ := cmd.Flags().GetString("to")
	onlyStr, _ := cmd.Flags().GetString("only")
	pitr, _ := cmd.Flags().GetString("point-in-time")
	decryptKey, _ := cmd.Flags().GetString("decrypt-key")
	yes, _ := cmd.Flags().GetBool("yes")
	confirmProject, _ := cmd.Flags().GetString("confirm-project")

	var only []string
	if onlyStr != "" {
		only = strings.Split(onlyStr, ",")
	}

	// pitr is read from flags but PointInTime was removed from RestoreOptions in v1.0.9
	// (PITR ships in v1.1.0 via pgbackrest integration). Guard against non-empty value.
	if pitr != "" {
		return fmt.Errorf("point-in-time restore is not available in this version; upgrade to v1.1.0 when released")
	}
	if err := confirmBackupRestoreWith(cmd, cfg, yes, confirmProject); err != nil {
		return err
	}

	opts := backup.RestoreOptions{
		BackupID:   args[0],
		ToDir:      toDir,
		Only:       only,
		DecryptKey: decryptKey,
		Yes:        yes,
	}

	// SIGINT and SIGTERM cancel ctx so Restore unwinds through its defers and
	// removes the decrypted dump; a second signal gets the default behaviour.
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { <-ctx.Done(); stop() }()
	if err := backup.Restore(ctx, cfg, opts); err != nil {
		return fmt.Errorf("backup restore: %w", err)
	}
	fmt.Println("Backup restored successfully.")
	return nil
}

// confirmBackupRestore and confirmBackupRestoreWith apply consent.Confirm, the
// rule both subcommands share, to the command's stdin and stderr.
func confirmBackupRestore(cmd *cobra.Command, cfg *config.Config, yes bool) error {
	return confirmBackupRestoreWith(cmd, cfg, yes, "")
}

func confirmBackupRestoreWith(cmd *cobra.Command, cfg *config.Config, yes bool, proof string) error {
	return consent.Confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), cfg, yes, proof)
}

// ── backup verify ──────────────────────────────────────────────────

var backupRestoreRemoteCmd = &cobra.Command{
	Use:   "restore-remote",
	Short: "Restore a backup directly from a remote URL",
	Long: `Stream and restore a backup from a remote destination without writing to disk.

The pipeline runs three concurrent stages:

  rclone cat <from> | age --decrypt | pg_restore

pg_restore runs in the postgres container into a scratch database that
replaces the live one only after it succeeds. Confirmation is the same as for
"nself backup restore" (--confirm-project <name> works without a prompt).

Examples:

  nself backup restore-remote --from s3:bucket/backup.sql.age --key ~/.age/key.txt
  nself backup restore-remote --from r2:bucket/backup.sql  # unencrypted`,
	RunE: dataEnv(runBackupRestoreRemote),
}

func runBackupRestoreRemote(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	from, _ := cmd.Flags().GetString("from")
	keyPath, _ := cmd.Flags().GetString("key")
	yes, _ := cmd.Flags().GetBool("yes")
	confirmProject, _ := cmd.Flags().GetString("confirm-project")
	if err := confirmBackupRestoreWith(cmd, cfg, yes, confirmProject); err != nil {
		return err
	}

	if err := backup.RestoreFromRemote(cmd.Context(), cfg, from, keyPath); err != nil {
		return fmt.Errorf("restore-remote: %w", err)
	}

	fmt.Println("Remote restore complete.")
	return nil
}

// ── init ────────────────────────────────────────────────────────────
