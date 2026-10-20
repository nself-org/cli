package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
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
	if projectDir == "" { // never write state into the working directory
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "entitlement refresh failed: no-project")
		return errs.Exit(errs.ExitUserError)
	}
	out, err := runScheduledRefresh(cmd.Context(), projectDir)
	doc := licenseRefreshData{Mode: "scheduled", Keys: []licenseRefreshKey{}}
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
		doc.TimerRemoved = true
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "entitlement refresh timer removed (%s)\n", out.Reason)
	case !out.Skipped && !out.Exp.IsZero():
		doc.EntitlementExpires = out.Exp.Format(time.RFC3339)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "entitlement refreshed exp=%s\n", out.Exp.Format(time.RFC3339))
	}
	setData(cmd, doc)
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
	setData(cmd, licenseRefreshData{Mode: "timer-preview", Keys: []licenseRefreshKey{}, Preview: text})
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
	scope := "--project '" + strings.ReplaceAll(projectDir, "'", `'\''`) + "'"
	if all {
		scope = "--all"
	}
	if res.RootRequired {
		return errs.ExitWithf(errs.ExitUserError, "the systemd timer units are root-owned; run: sudo nself license refresh --remove-timer %s", scope)
	}
	setData(cmd, licenseRefreshData{Mode: "remove-timer", Keys: []licenseRefreshKey{}, TimerRemoved: res.Removed})
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
		doc := licenseRefreshData{Mode: "keys", Keys: []licenseRefreshKey{}}
		for _, key := range keys {
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			result, err := license.RefreshCache(ctx, key)
			cancel()

			masked := license.MaskKey(key)
			if err != nil {
				doc.Keys = append(doc.Keys, licenseRefreshKey{Key: masked, Error: "refresh failed: " + licenseScrub(err.Error())})
				fmt.Fprintf(os.Stderr, "%s: refresh failed: %v\n", masked, licenseScrub(err.Error(), key))
				continue
			}
			if !result.Valid {
				doc.Keys = append(doc.Keys, licenseRefreshKey{Key: masked, Error: "invalid: " + licenseScrub(result.Message)})
				fmt.Fprintf(os.Stderr, "%s: invalid — %s\n", masked, licenseScrub(result.Message, key))
				continue
			}
			row := licenseRefreshKey{Key: masked, Refreshed: true, Tier: licenseScrub(result.Tier)}
			if !result.ExpiresAt.IsZero() {
				row.ExpiresAt = result.ExpiresAt.Format("2006-01-02")
			}
			doc.Keys = append(doc.Keys, row)
			fmt.Printf("%s: refreshed (tier: %s, expires: %s)\n",
				masked, row.Tier, result.ExpiresAt.Format("2006-01-02"))
			refreshed = true
		}
		setData(cmd, doc)
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
		if jsonEnvelopeOn(cmd) { // the signed cache holds the licence JWT: a document carries its summary only
			return exportCacheSummary(cmd)
		}
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

// timerAdapter maps internal/maintenance onto entitlement.Timers (entitlement may not import it).
func timerAdapter() entitlement.Timers {
	wrap := func(err error) error {
		switch {
		case errors.Is(err, maintenance.ErrUnsafePath):
			return fmt.Errorf("%w: %v", entitlement.ErrTimerUnsafePath, err)
		case errors.Is(err, maintenance.ErrNoBackend):
			return fmt.Errorf("%w: %v", entitlement.ErrTimerNoBackend, err)
		}
		return err
	}
	return entitlement.Timers{
		Ensure: func(dir, bin string) (entitlement.TimerInstall, error) {
			res, err := maintenance.Ensure(dir, bin)
			return entitlement.TimerInstall{Backend: res.Backend, Changed: res.Changed}, wrap(err)
		},
		Remove: func(dir string) (entitlement.TimerRemoval, error) {
			res, err := maintenance.Remove(dir)
			return entitlement.TimerRemoval{Removed: res.Removed, RootRequired: res.RootRequired}, err
		},
	}
}

func init() {
	entitlement.HostTimers = timerAdapter()
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
