package commands

// Purpose: the "nself backup config" and "nself backup status" subcommands
// and their RunE. Inputs are the cobra command/args; outputs are printed
// backup config/status or an error.
// Constraints: split out of backup_ops.go (CLI-R12) as a pure move, no behavior change.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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

	jsonOn := jsonEnvelopeOn(cmd)
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
		if jsonOn {
			return emitEnv(cmd, map[string]any{"install_cron": true, "dry_run": dryRun}, true)
		}
		if dryRun {
			return nil
		}
		fmt.Println("Systemd timers installed and enabled: nself-backup-{full,wal,prune,verify}.timer")
		return nil
	}

	format, _ := cmd.Flags().GetString("format")
	if jsonOn {
		format = "json"
	}
	output, err := backup.ConfigView(cfg, format)
	if err != nil {
		return err
	}
	output, err = withDestinationKinds(output, format, cfg.Backup.Remote)
	if err != nil {
		return err
	}
	if jsonOn {
		var view map[string]any
		if err := json.Unmarshal([]byte(output), &view); err != nil {
			return err
		}
		return emitEnv(cmd, view, true)
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
	format, _ := cmd.Flags().GetString("format")
	project, _ := cmd.Flags().GetString("project")
	hbTo, _ := cmd.Flags().GetString("heartbeat-to")
	var opts backup.OffboxOptions
	for _, f := range []struct {
		flag string
		dst  *time.Duration
	}{{"max-age", &opts.MaxAge}, {"max-drill-age", &opts.MaxDrillAge}} {
		flag, dst := f.flag, f.dst
		v, _ := cmd.Flags().GetString(flag)
		if v == "" {
			continue
		}
		d, err := backup.ParseAge(v)
		if err != nil {
			return fmt.Errorf("--%s: %w", flag, err)
		}
		*dst = d
	}

	// With --project and --heartbeat-to the command needs no project directory
	// (owner-machine jobs run from $HOME): only the offbox object is printed.
	var info *backup.StatusInfo
	if project == "" || hbTo == "" {
		cfg, err := loadProjectConfig()
		if err != nil {
			return err
		}
		if info, err = backup.Status(cfg); err != nil {
			return fmt.Errorf("backup status: %w", err)
		}
		if project == "" {
			project = cfg.ProjectName
		}
		if hbTo == "" {
			hbTo = cfg.Backup.HeartbeatRemote()
		}
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	// SIGINT and SIGTERM cancel the remote reads so their defers run.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { <-ctx.Done(); stop() }()
	off, offErr := backup.ReadOffbox(ctx, hbTo, project, opts, time.Now())
	if jsonEnvelopeOn(cmd) {
		// The document carries the state; a coded failure (E217-E219) keeps
		// its exit status without a second document.
		if err := emitEnv(cmd, newBackupStatusData(info, off), true); err != nil {
			return err
		}
		if offErr != nil {
			return codedExit(cmd, offErr)
		}
		return nil
	}
	output, err := backup.FormatStatusOffbox(info, off, format)
	if err != nil {
		return err
	}
	fmt.Print(output)
	// The state is printed first so a JSON consumer sees it; the coded error
	// (E217, E218, E219) sets the exit status.
	return offErr
}

func init() {
	backupStatusCmd.Flags().String("heartbeat-to", "", "Heartbeat remote to read backup.json and drill.json from (default: NSELF_BACKUP_HEARTBEAT_REMOTE)")
	backupStatusCmd.Flags().String("max-age", "", "Fail with E217 when the newest off-box backup is older (e.g. 26h)")
	backupStatusCmd.Flags().String("max-drill-age", "", "Fail with E218 when the newest restore drill is older or failed (e.g. 35d)")
	backupStatusCmd.Flags().String("project", "", "Project name for the heartbeat objects; with --heartbeat-to no project directory is needed")
}

// ── backup init-key ────────────────────────────────────────────────
