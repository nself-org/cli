// S9.T03 — `nself backup drill` cobra wiring.
//
// This file adds a single subcommand under the existing `backupCmd` parent
// (defined in backup_ops.go). The command is intentionally thin — all
// orchestration lives in internal/backup.Drill so the same logic powers the
// scripts/dr-drill.sh weekly cron + future Admin UI buttons.
package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nself-org/cli/internal/backup"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
)

var backupDrillCmd = &cobra.Command{
	Use:   "drill",
	Short: "Run a DR drill: restore latest backup into scratch DB and measure RTO",
	Long: `Run a disaster-recovery drill.

The drill restores the most recent backup (or the file passed via --file) into
a temporary scratch database, runs smoke checks against the critical np_*
tables (np_users, np_licenses, np_audit_log, np_plugins, np_billing), measures
total wall-clock RTO, and writes the structured result to
.nself/drill-log.json. Production data is never touched.

Default RTO target is 4 hours per STRAT-11; failures past that flip
result.RTOTargetMet to false but do not abort the drill. Pass --rto-hours 0 to
disable the gate entirely (the drill still records its duration).

With --from <remote> the drill is off-box: it downloads the newest (or --key)
<project>_stream_* backup through the destination interface, decrypts it with
--identity, restores it into a throwaway postgres:16-alpine container (random
name and password, no network, removed on every exit), counts rows per table
and checks every table the backup heartbeat estimated non-empty. The result is
written to <project>/drill.json on --heartbeat-to. The project database and
containers are never touched, and with --project no project directory is needed.

The doctor check OPS-DRILL-01 reads the drill log to enforce "drill within
last 7 days"; pair this command with a weekly cron via scripts/dr-drill.sh.`,
	RunE: runBackupDrill,
}

func runBackupDrill(cmd *cobra.Command, _ []string) error {
	if from, _ := cmd.Flags().GetString("from"); from != "" {
		return runBackupDrillRemote(cmd, from)
	}
	for _, f := range []string{"identity", "key", "heartbeat-to", "project"} {
		if v, _ := cmd.Flags().GetString(f); v != "" {
			return fmt.Errorf("--%s needs --from", f)
		}
	}
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	file, _ := cmd.Flags().GetString("file")
	rto, _ := cmd.Flags().GetFloat64("rto-hours")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	jsonOut, _ := cmd.Flags().GetBool("json")

	opts := database.DrillOptions{
		BackupFile:     file,
		RTOTargetHours: rto,
		DryRun:         dryRun,
		ProjectDir:     ".",
	}

	result, err := database.Drill(cmd.Context(), cfg, opts)
	if jsonEnvelopeOn(cmd) {
		// The outcome is the document even when the drill failed, so cron
		// parsers see it; the failure keeps its exit status.
		if eerr := emitEnv(cmd, backupDrillData{Local: &result}, false); eerr != nil {
			return eerr
		}
		if err != nil {
			return codedExit(cmd, err)
		}
		return nil
	}
	if jsonOut {
		// Always emit JSON when --json is set, even on error, so cron parsers
		// see the structured outcome (the err itself is the same string as
		// result.ErrorMessage in failure paths).
		data, mErr := json.MarshalIndent(result, "", "  ")
		if mErr == nil {
			fmt.Println(string(data))
		}
		return err
	}
	if err != nil {
		return fmt.Errorf("backup drill: %w", err)
	}

	verdict := "PASS"
	if !result.RTOTargetMet {
		verdict = "PASS (over RTO target)"
	}
	ui.Info(fmt.Sprintf("Backup drill: %s", verdict))
	ui.Dimmed(fmt.Sprintf("  Backup file:    %s", result.BackupFile))
	ui.Dimmed(fmt.Sprintf("  Duration:       %.1fs (restore=%.1fs verify=%.1fs smoke=%.1fs)",
		result.TotalDuration.Seconds(), result.RestoreSeconds, result.VerifySeconds, result.SmokeSeconds))
	ui.Dimmed(fmt.Sprintf("  Tables checked: %d", result.TablesChecked))
	ui.Dimmed(fmt.Sprintf("  Rows observed:  %d", result.RowsObserved))
	ui.Dimmed(fmt.Sprintf("  RTO target met: %v", result.RTOTargetMet))
	if len(result.MissingCriticalTables) > 0 {
		ui.Dimmed(fmt.Sprintf("  Critical tables not found by name: %v (set BACKUP_CRITICAL_TABLES if your schema uses different names)",
			result.MissingCriticalTables))
	}
	return nil
}

// runBackupDrillRemote restores the newest (or --key) backup of a remote into a
// throwaway container and writes drill.json to the heartbeat remote. With
// --project it needs no project directory.
func runBackupDrillRemote(cmd *cobra.Command, from string) error {
	project, _ := cmd.Flags().GetString("project")
	key, _ := cmd.Flags().GetString("key")
	identity, _ := cmd.Flags().GetString("identity")
	hbTo, _ := cmd.Flags().GetString("heartbeat-to")
	jsonOut, _ := cmd.Flags().GetBool("json")
	for _, f := range []string{"file", "dry-run"} {
		if cmd.Flags().Changed(f) {
			return fmt.Errorf("--%s cannot be used with --from", f)
		}
	}
	if project == "" {
		cfg, err := loadProjectConfig()
		if err != nil {
			return err
		}
		project = cfg.ProjectName
		if hbTo == "" {
			hbTo = cfg.Backup.HeartbeatRemote()
		}
	} else if hbTo == "" {
		hbTo = config.BackupConfig{}.HeartbeatRemote()
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	// SIGINT and SIGTERM cancel ctx so DrillRemote unwinds through its defers
	// and removes the throwaway container and the decrypted files. A second
	// signal after the first gets the default behaviour back.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { <-ctx.Done(); stop() }()
	res, err := backup.DrillRemote(ctx, backup.DrillRemoteOptions{
		Project: project, From: from, Key: key, Identity: identity, HeartbeatTo: hbTo,
	})
	if res != nil {
		if jsonEnvelopeOn(cmd) {
			if eerr := emitEnv(cmd, backupDrillData{Remote: newDrillHeartbeat(res.Heartbeat)}, false); eerr != nil {
				return eerr
			}
			if err != nil {
				return codedExit(cmd, err)
			}
			return nil
		}
		if jsonOut {
			if data, mErr := res.Heartbeat.Marshal(); mErr == nil {
				fmt.Print(string(data))
			}
		} else {
			printRemoteDrill(res, hbTo != "")
		}
	}
	return err
}

func printRemoteDrill(res *backup.DrillRemoteResult, hasHeartbeat bool) {
	hb := res.Heartbeat
	ui.Info(fmt.Sprintf("Backup drill (remote): %s", strings.ToUpper(hb.Result)))
	ui.Dimmed(fmt.Sprintf("  Backup object:  %s (%d bytes, encrypted=%v)", hb.BackupKey, hb.Bytes, hb.Encrypted))
	ui.Dimmed(fmt.Sprintf("  Duration:       %.1fs", res.Duration.Seconds()))
	for _, t := range res.Tables {
		ui.Dimmed(fmt.Sprintf("  %-40s %d rows", t, hb.RestoredRows[t]))
	}
	for _, w := range res.Warnings {
		ui.Dimmed("  Warning:        " + w)
	}
	if len(hb.Mismatches) > 0 {
		ui.Dimmed(fmt.Sprintf("  Mismatches:     %s", strings.Join(hb.Mismatches, ", ")))
	}
	switch {
	case res.HeartbeatWritten:
		ui.Dimmed("  drill.json written to the heartbeat remote.")
	case !hasHeartbeat:
		ui.Dimmed("  No heartbeat remote configured: drill.json not written.")
	}
}

func init() {
	backupDrillCmd.Flags().String("from", "", "Restore the newest backup of this remote (rclone remote, path://, host://) into a throwaway container")
	backupDrillCmd.Flags().String("identity", "", "age identity file that decrypts the backup (with --from)")
	backupDrillCmd.Flags().String("key", "", "Backup object name to drill instead of the newest (with --from)")
	backupDrillCmd.Flags().String("heartbeat-to", "", "Heartbeat remote that receives drill.json (with --from; default NSELF_BACKUP_HEARTBEAT_REMOTE)")
	backupDrillCmd.Flags().String("project", "", "Project name for the backup objects and heartbeat; with --from no project directory is needed")
	backupDrillCmd.Flags().String("file", "", "Backup file to drill against (default: most recent)")
	backupDrillCmd.Flags().Float64("rto-hours", 4.0, "RTO target in hours; 0 disables the gate")
	backupDrillCmd.Flags().Bool("dry-run", false, "Validate inputs and exit without restoring")
	backupDrillCmd.Flags().Bool("json", false, "Emit DrillResult as JSON to stdout")

	backupCmd.AddCommand(backupDrillCmd)
}
