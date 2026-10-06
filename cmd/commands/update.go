package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/ui"
	"github.com/nself-org/cli/internal/version"

	"github.com/spf13/cobra"
)

// githubRelease represents the minimal fields from the GitHub releases API.
type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

const (
	githubLatestReleaseURL = "https://api.github.com/repos/nself-org/cli/releases/latest"
	githubReleaseByTagURL  = "https://api.github.com/repos/nself-org/cli/releases/tags/%s"
	githubDownloadBaseURL  = "https://github.com/nself-org/cli/releases/download"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the nSelf CLI and admin UI",
	Long: `Check for and install updates to the nSelf CLI binary and the
admin Docker image. Use --check to see if an update is available
without installing it.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		check, _ := cmd.Flags().GetBool("check")
		cliOnly, _ := cmd.Flags().GetBool("cli")
		adminOnly, _ := cmd.Flags().GetBool("admin")
		force, _ := cmd.Flags().GetBool("force")
		restart, _ := cmd.Flags().GetBool("restart")
		targetVersion, _ := cmd.Flags().GetString("version")

		current := version.GetVersion()

		// Resolve the target version: explicit flag > latest from GitHub.
		var latest, releaseURL string
		if targetVersion != "" {
			tag := targetVersion
			if !strings.HasPrefix(tag, "v") {
				tag = "v" + tag
			}
			var err error
			latest, releaseURL, err = fetchReleaseByTag(tag)
			if err != nil {
				return fmt.Errorf("failed to fetch release %s: %w", targetVersion, err)
			}
		} else {
			var err error
			latest, releaseURL, err = fetchLatestRelease()
			if err != nil {
				return fmt.Errorf("failed to check for updates: %w", err)
			}
		}

		upToDate := normalizeVersion(current) == normalizeVersion(latest)

		if check {
			return printCheckResult(current, latest, releaseURL, upToDate)
		}

		if upToDate && !force {
			ui.Info(fmt.Sprintf("Already on latest version (%s)", current))
			// Return a sentinel so the caller can exit non-zero.
			return &alreadyLatestError{version: current}
		}

		// Determine what to update.
		updateCLI := !adminOnly
		updateAdmin := !cliOnly

		if updateCLI {
			ui.Info(fmt.Sprintf("Updating CLI: %s -> %s", current, latest))
			if err := selfUpdate(latest); err != nil {
				return fmt.Errorf("CLI update failed: %w", err)
			}
			ui.Success(fmt.Sprintf("CLI updated: %s -> %s", current, latest))
		}

		if updateAdmin {
			// The admin reference comes from the image lock: legacy mode is the
			// historical nself/nself-admin:latest, lock mode the pinned digest.
			adminImage := compose.ImageRef("admin", "")
			ui.Info("Pulling admin image: " + adminImage)
			pullCmd := exec.Command("docker", "pull", adminImage)
			pullCmd.Stdout = os.Stdout
			pullCmd.Stderr = os.Stderr
			if err := pullCmd.Run(); err != nil {
				return fmt.Errorf("pulling admin image: %w", err)
			}
			ui.Success("Admin image updated.")
		}

		if restart {
			projectDir, projErr := resolveProjectDir()
			if projErr != nil {
				ui.Warn("Cannot restart services: no nself project found in current directory")
			} else {
				ui.Info("Restarting services...")
				restartExec := exec.Command("docker", "compose", "restart")
				restartExec.Dir = projectDir
				restartExec.Stdout = os.Stdout
				restartExec.Stderr = os.Stderr
				if err := restartExec.Run(); err != nil {
					return fmt.Errorf("restarting services: %w", err)
				}
				ui.Success("Services restarted.")
			}
		}

		return nil
	},
}

// alreadyLatestError is returned (non-nil) when the binary is already on the
// latest version. Cobra will print the message and exit non-zero.
type alreadyLatestError struct{ version string }

func (e *alreadyLatestError) Error() string {
	return fmt.Sprintf("Already on latest version (%s)", e.version)
}

func init() {
	updateCmd.Flags().Bool("check", false, "Check for updates without installing")
	updateCmd.Flags().Bool("cli", false, "Only update the CLI binary")
	updateCmd.Flags().Bool("admin", false, "Only update the admin UI")
	updateCmd.Flags().Bool("force", false, "Force update even if already up to date")
	updateCmd.Flags().Bool("restart", false, "Restart services after update")
	updateCmd.Flags().String("version", "", "Download a specific version (e.g. v1.2.3)")
	RootCmd.AddCommand(updateCmd)
}
