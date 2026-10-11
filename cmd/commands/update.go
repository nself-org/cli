package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/lock"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/reconcile"
	"github.com/nself-org/cli/internal/ui"
	"github.com/nself-org/cli/internal/version"

	"github.com/spf13/cobra"
)

// githubRelease represents the minimal fields from the GitHub releases API.
type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// The release endpoints are variables so the cmd/commands test binary can point
// them at a local stub (TestMain). A test that reaches selfUpdate with the real
// URLs would download the published release and overwrite the running test
// executable.
var (
	githubLatestReleaseURL = "https://api.github.com/repos/nself-org/cli/releases/latest"
	githubReleaseByTagURL  = "https://api.github.com/repos/nself-org/cli/releases/tags/%s"
	githubDownloadBaseURL  = "https://github.com/nself-org/cli/releases/download"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the nSelf CLI and admin UI",
	Long: `Check for and install updates to the nSelf CLI binary and the
admin Docker image. Use --check to see if an update is available
without installing it.

Use --plan to see the pin diff of nself.lock.yaml (CLI, images, plugins) and
the upgrade steps without changing anything, and --yes to apply it.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if handled, err := runUpdatePlanMode(cmd); handled {
			return err
		}
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

// updateDeps is everything `update --plan|--yes` reaches outside the project
// files, so tests substitute fixtures (P7-LIVE-10).
type updateDeps struct {
	runningVersion func() string
	// latestRelease resolves the CLI target: "" is the latest release.
	latestRelease func(target string) (tag, url string, err error)
	registry      func(ctx context.Context) (*plugin.Registry, error)
	writeLock     func(dir string, l lock.Lock) error
	updatePlugin  func(ctx context.Context, dir, slug string) error
	build         func(ctx context.Context, dir string) error
	selfUpdate    func(tag string) error
}

// newUpdateDeps is a variable so tests can install fixtures.
var newUpdateDeps = func() updateDeps {
	return updateDeps{
		runningVersion: version.GetVersion,
		latestRelease: func(target string) (string, string, error) {
			if target == "" {
				return fetchLatestRelease()
			}
			return fetchReleaseByTag(target)
		},
		registry: func(ctx context.Context) (*plugin.Registry, error) {
			return plugin.FetchRegistry(ctx, "", plugin.DefaultCacheDir())
		},
		writeLock: writeLockFile,
		updatePlugin: func(ctx context.Context, dir, slug string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			return plugin.Update(ctx, cfg, slug, resolvePluginDir())
		},
		build: func(ctx context.Context, dir string) error {
			_, _, err := reconcile.ApplyBuild(ctx, reconcile.Request{
				ProjectDir: dir, Command: reconcile.CmdBuild, Trigger: reconcile.Trigger{Kind: reconcile.TriggerBuild},
			}, reconcile.ApplyOptions{Yes: true})
			return err
		},
		selfUpdate: selfUpdate,
	}
}

func init() {
	addUpdatePlanFlags(updateCmd)
	updateCmd.Flags().Bool("check", false, "Check for updates without installing")
	updateCmd.Flags().Bool("cli", false, "Only update the CLI binary")
	updateCmd.Flags().Bool("admin", false, "Only update the admin UI")
	updateCmd.Flags().Bool("force", false, "Force update even if already up to date")
	updateCmd.Flags().Bool("restart", false, "Restart services after update")
	updateCmd.Flags().String("version", "", "Download a specific version (e.g. v1.2.3)")
	RootCmd.AddCommand(updateCmd)
}
