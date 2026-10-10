package commands

// Purpose: the "nself backup verify" and "nself backup prune" subcommands and
// their RunE. Inputs are the cobra command/args; outputs are verification
// results or pruned backups, or an error.
// Constraints: split out of backup_ops.go (CLI-R12) as a pure move, no behavior change.

import (
	"fmt"

	"github.com/nself-org/cli/internal/backup"
	"github.com/spf13/cobra"
)

var backupVerifyCmd = &cobra.Command{
	Use:   "verify <backup-id|latest>",
	Short: "Verify backup integrity",
	Args:  cobra.ExactArgs(1),
	RunE:  dataEnv(runBackupVerify),
}

func runBackupVerify(cmd *cobra.Command, args []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	restoreTest, _ := cmd.Flags().GetBool("restore-test")
	cleanup, _ := cmd.Flags().GetBool("cleanup")
	keep, _ := cmd.Flags().GetBool("keep")

	result, err := backup.Verify(cmd.Context(), cfg, backup.VerifyOptions{
		BackupID:    args[0],
		RestoreTest: restoreTest,
		Cleanup:     cleanup,
		Keep:        keep,
	})
	if err != nil {
		return fmt.Errorf("backup verify: %w", err)
	}

	setData(cmd, *result)
	if result.Verified {
		fmt.Printf("Backup %s verified (%s, %s).\n", result.BackupID, result.Method, result.Duration)
	} else {
		fmt.Printf("Backup %s verification FAILED: %s\n", result.BackupID, result.Details)
	}
	return nil
}

// ── backup prune ───────────────────────────────────────────────────

var backupPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove old backups by retention policy",
	RunE:  runBackupPrune,
}

func runBackupPrune(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	keepDaily, _ := cmd.Flags().GetInt("keep-daily")
	keepWeekly, _ := cmd.Flags().GetInt("keep-weekly")
	keepMonthly, _ := cmd.Flags().GetInt("keep-monthly")
	format, _ := cmd.Flags().GetString("format")

	result, err := backup.Prune(cfg, backup.PruneOptions{
		DryRun:      dryRun,
		KeepDaily:   keepDaily,
		KeepWeekly:  keepWeekly,
		KeepMonthly: keepMonthly,
	})
	if err != nil {
		return fmt.Errorf("backup prune: %w", err)
	}

	if jsonEnvelopeOn(cmd) {
		return emitEnv(cmd, backup.BuildPruneReport(result, keepDaily), true)
	}
	if format == "json" {
		return backup.FormatPruneJSON(cmd.OutOrStdout(), result, keepDaily)
	}

	prefix := ""
	if result.DryRun {
		prefix = "(dry-run) "
	}
	fmt.Printf("%sKept %d backups, pruned %d.\n", prefix, len(result.Kept), len(result.Pruned))
	return nil
}

// ── backup config ──────────────────────────────────────────────────
