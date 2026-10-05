package commands

// Purpose: the "nself backup config" and "nself backup status" subcommands
// and their RunE. Inputs are the cobra command/args; outputs are printed
// backup config/status or an error.
// Constraints: split out of backup_ops.go (CLI-R12) as a pure move, no behavior change.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/backup"
	"github.com/nself-org/cli/internal/backup/destinations"
	"github.com/spf13/cobra"
)

var backupConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "View backup configuration",
	RunE:  runBackupConfig,
}

func runBackupConfig(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	installCron, _ := cmd.Flags().GetBool("install-cron")
	if installCron {
		fullAt, _ := cmd.Flags().GetString("full-at")
		walEvery, _ := cmd.Flags().GetString("wal-every")
		pruneAt, _ := cmd.Flags().GetString("prune-at")
		verifyOn, _ := cmd.Flags().GetString("verify-on")
		verifyAt, _ := cmd.Flags().GetString("verify-at")
		remote, _ := cmd.Flags().GetString("remote")
		unitDir, _ := cmd.Flags().GetString("unit-dir")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		opts := backup.SystemdInstallOptions{
			FullAt:      fullAt,
			WALEvery:    walEvery,
			PruneAt:     pruneAt,
			VerifyOnDay: verifyOn,
			VerifyAt:    verifyAt,
			Remote:      remote,
			UnitDir:     unitDir,
			DryRun:      dryRun,
		}
		if err := backup.InstallSystemdUnits(cfg, opts); err != nil {
			return fmt.Errorf("install-cron: %w", err)
		}
		if dryRun {
			return nil
		}
		fmt.Println("Systemd timers installed and enabled: nself-backup-{full,wal,prune,verify}.timer")
		return nil
	}

	format, _ := cmd.Flags().GetString("format")
	output, err := backup.ConfigView(cfg, format)
	if err != nil {
		return err
	}
	output, err = withDestinationKinds(output, format, cfg.Backup.Remote)
	if err != nil {
		return err
	}
	fmt.Print(output)
	return nil
}

// withDestinationKinds adds the supported destination kinds and the kind the
// configured remote selects ("none" when no remote is set) to the output of
// backup.ConfigView. JSON output gains the keys "destination" and
// "destination_kinds"; text output gains a Destinations section.
func withDestinationKinds(output, format, remote string) (string, error) {
	configured := destinations.KindOf(remote)
	if configured == "" {
		configured = "none"
	}
	kinds := destinations.Kinds()
	if format == "json" {
		var view map[string]interface{}
		if err := json.Unmarshal([]byte(output), &view); err != nil {
			return "", err
		}
		view["destination"] = configured
		view["destination_kinds"] = kinds
		data, err := json.MarshalIndent(view, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	var sb strings.Builder
	sb.WriteString(output)
	fmt.Fprintf(&sb, "Destinations (configured: %s):\n", configured)
	for _, k := range kinds {
		mark := " "
		if k.Kind == configured {
			mark = "*"
		}
		fmt.Fprintf(&sb, "  %s %-7s %s\n", mark, k.Kind, k.Example)
	}
	return sb.String(), nil
}

// ── backup status ──────────────────────────────────────────────────

var backupStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show backup subsystem status",
	RunE:  runBackupStatus,
}

func runBackupStatus(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	format, _ := cmd.Flags().GetString("format")
	info, err := backup.Status(cfg)
	if err != nil {
		return fmt.Errorf("backup status: %w", err)
	}

	output, err := backup.FormatStatus(info, format)
	if err != nil {
		return err
	}
	fmt.Print(output)
	return nil
}

// ── backup init-key ────────────────────────────────────────────────
