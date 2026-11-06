package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/backup"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/promote"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var promoteCmd = &cobra.Command{
	Use:   "promote <source> <target>",
	Short: "Promote one environment to another (e.g. staging to prod)",
	Long: `Promote configurations, migrations, and services from one environment to another.

Examples:
  nself promote staging prod --dry-run    # Preview changes
  nself promote staging prod --approve-id TICKET-123 --confirm prod`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// compat.V15(P7-DEPL-15): local internal/promote implementation -> deploy promote control-plane engine
		if compat.V15() {
			old := "promote"
			if strings.Contains(cmd.CommandPath(), "deploy promote-env") {
				old = "deploy promote-env"
			}
			emitCanonWarning(os.Stderr, os.Args[1:], []canonNote{{Old: old, New: "deploy promote", RemovalAt: "v1.6.0"}}, "", "")
			return runPromoteEnvironments(cmd, args[0], args[1])
		}
		source := args[0]
		target := args[1]
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		approveID, _ := cmd.Flags().GetString("approve-id")
		confirm, _ := cmd.Flags().GetString("confirm")
		jsonOut, _ := cmd.Flags().GetBool("json")

		cwd, _ := os.Getwd()
		ctx := cmd.Context()

		if dryRun {
			result, err := promote.DryRun(ctx, cwd, source, target)
			if err != nil {
				return err
			}

			if jsonOut {
				data, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			ui.CommandHeader("Promotion Dry Run", fmt.Sprintf("%s → %s", source, target))

			if len(result.EnvVars) > 0 {
				ui.Section("Environment Variables")
				for _, d := range result.EnvVars {
					if d.Status == "same" {
						continue
					}
					fmt.Printf("  [%s] %s: %s → %s\n", d.Status, d.Key, d.SourceValue, d.TargetValue)
				}
			}

			if len(result.Migrations) > 0 {
				ui.Section("Migrations")
				for _, d := range result.Migrations {
					if d.Status == "same" {
						continue
					}
					fmt.Printf("  [%s] %s\n", d.Status, d.Key)
				}
			}

			return nil
		}

		// Production safety gate
		if target == "prod" && confirm != "prod" {
			return fmt.Errorf("promoting to prod requires --confirm prod")
		}

		if approveID == "" {
			return fmt.Errorf("--approve-id is required for non-dry-run promotions")
		}

		record, err := promote.Execute(ctx, cwd, source, target, approveID)
		if err != nil {
			ui.Error(fmt.Sprintf("Promotion failed: %v", err))
			if record != nil && record.BackupTag != "" {
				ui.Warn(fmt.Sprintf("Rollback available: nself promote rollback --tag %s", record.BackupTag))
			}
			return err
		}

		ui.Success(fmt.Sprintf("Promotion %s → %s completed (id: %s)", source, target, record.ID))
		return nil
	},
}

var v14Backups = func() (refs []promote.BackupRef, err error) {
	cfg, err := loadProjectConfig()
	if err != nil {
		return nil, err
	}
	entries, err := backup.List(cfg, backup.ListOptions{})
	for _, e := range entries {
		refs = append(refs, promote.BackupRef{ID: e.ID, Tag: e.Tag, Date: e.Date})
	}
	return refs, err
}

func rollbackV14(cmd *cobra.Command, workdir, target string) error {
	tag, _ := cmd.Flags().GetString("tag")
	typed, _ := cmd.Flags().GetString("confirm-project")
	inv, _ := controlplane.Load(workdir)
	project, err := rollbackProject(typed, workdir, target, true, target == "" || prodClassFn(inv, target))
	if err != nil {
		return err
	}
	return promote.RollbackProject(cmd.Context(), workdir, project, tag, v14Backups)
}

var promoteRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Rollback to pre-promotion backup",
	RunE: func(cmd *cobra.Command, args []string) error {
		// compat.V15(P7-DEPL-15): local promote snapshot -> verified remote promote record
		if compat.V15() {
			tag, _ := cmd.Flags().GetString("tag")
			if tag != "" {
				return errs.New("E401", "v1.5 promote rollback restores the latest promote record; --tag is unsupported")
			}
			return runPromoteRecordRollback(cmd)
		}
		cwd, _ := os.Getwd()
		if err := rollbackV14(cmd, cwd, ""); err != nil {
			return err
		}

		ui.Success("Rollback completed")
		return nil
	},
}

func init() {
	promoteCmd.Flags().Bool("dry-run", false, "Preview changes without applying")
	promoteCmd.Flags().String("approve-id", "", "Approval ticket ID")
	promoteCmd.Flags().String("confirm", "", "Confirmation target (required for prod)")
	promoteCmd.Flags().Bool("json", false, "JSON output")
	promoteCmd.Flags().Bool("yes", false, "Confirm promotion to a production-class target")

	promoteRollbackCmd.Flags().String("tag", "", "Specific backup tag to restore (default: latest pre-promote)")
	promoteRollbackCmd.Flags().Bool("yes", false, "Confirm restoring a production-class database")
	promoteRollbackCmd.Flags().String("confirm-project", "", "Project name passed to backup restore (required to restore a production database)")

	RootCmd.AddCommand(promoteCmd)
	promoteCmd.AddCommand(promoteRollbackCmd)
}
