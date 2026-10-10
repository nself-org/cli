package commands

// Purpose: the "nself backup create" and "nself backup list" subcommands and
// their RunE. Inputs are the cobra command/args; outputs are a created backup
// or a printed backup listing, or an error.
// Constraints: split out of backup_ops.go (CLI-R12) as a pure move, no behavior change.

import (
	"fmt"
	"time"

	"github.com/nself-org/cli/internal/backup"
	"github.com/spf13/cobra"
)

var backupCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new backup",
	RunE:  dataEnv(runBackupCreate),
}

func runBackupCreate(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	backupType, _ := cmd.Flags().GetString("type")
	remote, _ := cmd.Flags().GetString("remote")
	encrypt, _ := cmd.Flags().GetBool("encrypt")
	noEncrypt, _ := cmd.Flags().GetBool("no-encrypt")
	tag, _ := cmd.Flags().GetString("tag")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	opts := backup.CreateOptions{
		Type:      backup.BackupType(backupType),
		Remote:    remote,
		Encrypt:   encrypt,
		NoEncrypt: noEncrypt,
		Tag:       tag,
		DryRun:    dryRun,
	}

	if err := backup.Create(cmd.Context(), cfg, opts); err != nil {
		return fmt.Errorf("backup create: %w", err)
	}
	fmt.Println("Backup created successfully.")
	return nil
}

// ── backup list ────────────────────────────────────────────────────

var backupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available backups",
	RunE:  runBackupList,
}

func runBackupList(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	remote, _ := cmd.Flags().GetString("remote")
	env, _ := cmd.Flags().GetString("env")
	sinceStr, _ := cmd.Flags().GetString("since")
	format, _ := cmd.Flags().GetString("format")

	var since time.Duration
	if sinceStr != "" {
		since, err = time.ParseDuration(sinceStr)
		if err != nil {
			return fmt.Errorf("invalid --since duration: %w", err)
		}
	}

	entries, err := backup.List(cfg, backup.ListOptions{
		Remote: remote,
		Env:    env,
		Since:  since,
		Format: format,
	})
	if err != nil {
		return fmt.Errorf("backup list: %w", err)
	}

	if jsonEnvelopeOn(cmd) {
		if entries == nil {
			entries = []backup.BackupEntry{}
		}
		return emitEnv(cmd, entries, true)
	}
	if len(entries) == 0 {
		fmt.Println("No backups found.")
		return nil
	}

	output, err := backup.FormatList(entries, format)
	if err != nil {
		return err
	}
	fmt.Print(output)
	return nil
}

// ── backup restore ─────────────────────────────────────────────────
