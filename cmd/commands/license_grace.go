package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/license/entitlement"
	"github.com/nself-org/cli/internal/maintenance"

	"github.com/spf13/cobra"
)

// Seams for tests: the scheduled refresh and the by-hand timer removal.
var (
	runScheduledRefresh = entitlement.RunScheduled
	removeTimers        = func(projectDir string, all bool) (maintenance.RemoveResult, error) {
		if all {
			return maintenance.RemoveAll()
		}
		return maintenance.Remove(projectDir)
	}
)

// licenseProject resolves --project, else the project around the working directory ("" outside one).
func licenseProject(cmd *cobra.Command) string {
	if dir, _ := cmd.Flags().GetString("project"); dir != "" {
		return dir
	}
	if cwd, err := os.Getwd(); err == nil {
		if root, err := config.FindNSelfRoot(cwd); err == nil {
			return root
		}
	}
	return ""
}

// scheduledRefresh is the timer's run: one line at most, exit 78 when only root can finish removal.
func scheduledRefresh(cmd *cobra.Command, projectDir string) error {
	out, err := runScheduledRefresh(cmd.Context(), projectDir)
	var failed *entitlement.RefreshError
	switch {
	case errors.As(err, &failed):
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), failed.Error())
		return errs.Exit(errs.ExitUserError)
	case err != nil:
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "entitlement refresh failed: internal")
		return errs.Exit(errs.ExitUserError)
	case out.RootRequired:
		return errs.Exit(maintenance.ExitGone)
	case out.Removed:
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "entitlement refresh timer removed (%s)\n", out.Reason)
	case !out.Skipped && !out.Exp.IsZero():
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "entitlement refreshed exp=%s\n", out.Exp.Format(time.RFC3339))
	}
	return nil
}

// previewTimer prints the units Ensure would write for this project and binary.
func previewTimer(cmd *cobra.Command, projectDir string) error {
	exe, err := os.Executable()
	if err != nil || projectDir == "" {
		return errs.ExitWithf(errs.ExitUserError, "run this inside a project or pass --project DIR")
	}
	text, err := maintenance.Preview(projectDir, exe)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprint(cmd.OutOrStdout(), text)
	return nil
}

// removeTimerByHand implements `license refresh --remove-timer [--project DIR | --all]`.
func removeTimerByHand(cmd *cobra.Command, projectDir string, all bool) error {
	if !all && projectDir == "" {
		return errs.ExitWithf(errs.ExitUserError, "run this inside a project, or pass --project DIR or --all")
	}
	res, err := removeTimers(projectDir, all)
	if err != nil {
		return err
	}
	scope := "--project " + projectDir
	if all {
		scope = "--all"
	}
	if res.RootRequired {
		return errs.ExitWithf(errs.ExitUserError, "the systemd timer units are root-owned; run: sudo nself license refresh --remove-timer %s", scope)
	}
	if res.Removed {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "entitlement refresh timer removed")
	} else {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no entitlement refresh timer found")
	}
	return nil
}

var licenseRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Force-refresh license validation against ping.nself.org",
	Long: `Force-refresh license validation against ping.nself.org.

In a v1.5 project with licensed plugins it also rewrites .nself/state/entitlement.jwt once the
new token verifies. The unattended host timer runs 'nself license refresh --project DIR --quiet'
every 6 hours. --remove-timer removes that timer by hand (--all: every project of this user).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if remove, _ := cmd.Flags().GetBool("remove-timer"); remove {
			all, _ := cmd.Flags().GetBool("all")
			return removeTimerByHand(cmd, licenseProject(cmd), all)
		}
		if dry, _ := cmd.Flags().GetBool("install-timer-dry-run"); dry {
			return previewTimer(cmd, licenseProject(cmd))
		}
		quiet, _ := cmd.Flags().GetBool("quiet")
		// compat.V15(P7-PLUG-64): refresh only the licence cache -> scheduled mode also rewrites entitlement.jwt and manages the timer
		if dir, _ := cmd.Flags().GetString("project"); compat.V15() && (quiet || dir != "") {
			return scheduledRefresh(cmd, licenseProject(cmd))
		}
		keys := license.CollectLicenseKeys()
		if len(keys) == 0 {
			return fmt.Errorf("no license key configured — run 'nself license add <key>' first")
		}

		refreshed := false
		for _, key := range keys {
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			result, err := license.RefreshCache(ctx, key)
			cancel()

			masked := license.MaskKey(key)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: refresh failed: %v\n", masked, err)
				continue
			}
			if !result.Valid {
				fmt.Fprintf(os.Stderr, "%s: invalid — %s\n", masked, result.Message)
				continue
			}
			fmt.Printf("%s: refreshed (tier: %s, expires: %s)\n",
				masked, result.Tier, result.ExpiresAt.Format("2006-01-02"))
			refreshed = true
		}
		// compat.V15(P7-PLUG-64): refresh leaves entitlement.jwt alone -> a verified refresh rewrites it
		if dir := licenseProject(cmd); refreshed && compat.V15() && dir != "" {
			return publishEntitlement(cmd, dir)
		}
		return nil
	},
}

var licenseExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export signed license cache for air-gap transfer",
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := license.ExportCache()
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	},
}

var licenseImportCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import a previously exported license cache file",
	Long: `Import a previously exported license cache file.

Pass --force when importing an unsigned cache entry while NSELF_LICENSE_SKIP_VERIFY=1
is set. Without --force, the skip-verify mode is rejected to prevent accidental bypass.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")

		// Signal --force acknowledgment to the library layer via env var.
		// This is the approved path for NSELF_LICENSE_SKIP_VERIFY in the import command.
		if force && os.Getenv("NSELF_LICENSE_SKIP_VERIFY") == "1" {
			if err := os.Setenv("NSELF_LICENSE_SKIP_VERIFY_FORCE", "1"); err != nil {
				return fmt.Errorf("setting skip-verify force flag: %w", err)
			}
			fmt.Fprintf(os.Stderr, "warning: skip-verify mode active (--force acknowledged)\n")
		}

		data, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("reading cache file: %w", err)
		}
		if err := license.ImportCache(data); err != nil {
			return err
		}
		fmt.Println("License cache imported successfully.")
		// compat.V15(P7-PLUG-64): import ignores the JWT -> an import carrying a JWT rewrites entitlement.jwt
		if dir := licenseProject(cmd); compat.V15() && dir != "" {
			return publishEntitlement(cmd, dir)
		}
		return nil
	},
}

// publishEntitlement rewrites the entitlement file from the verified cache; failure only warns.
func publishEntitlement(cmd *cobra.Command, projectDir string) error {
	if err := entitlement.PublishFromCache(projectDir); err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: entitlement file not updated: %v\n", err)
	}
	return nil
}

func init() {
	licenseRefreshCmd.Flags().String("project", "", "Project directory (default: the project around the working directory)")
	licenseRefreshCmd.Flags().Bool("quiet", false, "Print at most one line (used by the unattended refresh timer)")
	licenseRefreshCmd.Flags().Bool("remove-timer", false, "Remove the unattended entitlement refresh timer and exit")
	licenseRefreshCmd.Flags().Bool("all", false, "With --remove-timer: remove the timers of every project of this user")
	licenseRefreshCmd.Flags().Bool("install-timer-dry-run", false, "Print the timer that would be installed and exit")
	_ = licenseRefreshCmd.Flags().MarkHidden("install-timer-dry-run")
	licenseImportCmd.Flags().Bool("force", false, "Required when NSELF_LICENSE_SKIP_VERIFY=1; explicit acknowledgment of skipped signature verification")
	licenseCmd.AddCommand(licenseRefreshCmd)
	licenseCmd.AddCommand(licenseExportCmd)
	licenseCmd.AddCommand(licenseImportCmd)
}
