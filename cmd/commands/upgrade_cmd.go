package commands

// Purpose: The `nself upgrade` cobra command itself — a large RunE closure
// that resolves the requested channel/version, downloads and validates the
// new binary (via upgrade_binary.go's helpers), backs up and swaps in the
// new binary, and rolls back on failure — plus its flag registration.
// Split out of upgrade.go (CLI-R12) to keep the command declaration and
// wiring in its own file under the size cap, separate from the
// channel-config helpers (upgrade.go) and binary/URL helpers
// (upgrade_binary.go) it calls.
// Inputs: the cobra.Command + flags (--channel, --version, --binary-url,
// --binary-sha256, --force, --check).
// Outputs: an upgraded (or rolled-back) nself binary; printed progress.
// Constraints: pure move — no behavior changes.

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/ui"
	"github.com/nself-org/cli/internal/version"

	"github.com/spf13/cobra"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade the nSelf CLI (detects install method)",
	Long: `Upgrade the nSelf CLI binary. Detects install method (Homebrew, direct download,
system package) and uses the appropriate upgrade strategy.

  nself upgrade                          # Upgrade to latest stable
  nself upgrade --check                  # Check without installing
  nself upgrade --channel canary         # Switch to canary channel
  nself upgrade --version 1.2.3         # Pin to specific version
  nself upgrade --rollback               # Revert to previous version
  nself upgrade --binary-url <url>       # Install from a specific URL (operators)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		check, _ := cmd.Flags().GetBool("check")
		channel, _ := cmd.Flags().GetString("channel")
		targetVersion, _ := cmd.Flags().GetString("version")
		doRollback, _ := cmd.Flags().GetBool("rollback")
		binaryURL, _ := cmd.Flags().GetString("binary-url")
		binarySHA, _ := cmd.Flags().GetString("binary-sha256")

		setData(cmd, newUpdateUpgradeData(check, doRollback, binaryURL != "", targetVersion))
		if doRollback {
			return rollbackBinary()
		}

		// --binary-url: operator-pinned direct download path.
		// Skips the GitHub releases API entirely and downloads from the
		// provided URL.  All other upgrade guards (backup, checksum,
		// atomic swap) still apply.
		if binaryURL != "" {
			if err := validateBinaryURL(binaryURL); err != nil {
				return err
			}

			binarySHA = strings.TrimSpace(strings.ToLower(binarySHA))
			if binarySHA != "" {
				if err := validateBinarySHA256(binarySHA); err != nil {
					return err
				}
			}

			method := detectInstallMethod()
			if method == "homebrew" {
				ui.Warn("This nSelf binary was installed via Homebrew.")
				ui.Warn("Using --binary-url with a Homebrew install may break Homebrew's tracking of the formula.")
				ui.Warn("Consider upgrading through Homebrew instead unless you have a specific reason to bypass it.")
			}

			// Backup before swapping.
			if backupPath, err := backupBinary(); err != nil {
				ui.Warn(fmt.Sprintf("Could not backup current binary: %v", err))
			} else {
				ui.Dimmed(fmt.Sprintf("Backup saved to %s", backupPath))
			}

			ui.Info(fmt.Sprintf("Downloading binary from %s", binaryURL))
			var upgradeErr error
			if binarySHA != "" {
				ui.Info("Verifying with operator-supplied --binary-sha256")
				upgradeErr = selfUpdateFromURLWithSHA(binaryURL, binarySHA)
			} else {
				checksumURL := checksumURLFromBinaryURL(binaryURL)
				ui.Info(fmt.Sprintf("Fetching checksum from %s", checksumURL))
				upgradeErr = selfUpdateFromURL(binaryURL, checksumURL)
			}
			if upgradeErr != nil {
				ui.Error(fmt.Sprintf("Upgrade failed: %v", upgradeErr))
				ui.Info("Run 'nself upgrade --rollback' to restore the previous version")
				return upgradeErr
			}

			ui.Success("Binary installed from custom URL")
			return nil
		}

		// --binary-sha256 without --binary-url is a misuse; reject early.
		if binarySHA != "" {
			return fmt.Errorf("--binary-sha256 requires --binary-url")
		}

		// Persist channel choice if provided
		if channel != "" {
			if channel != "stable" && channel != "canary" {
				return fmt.Errorf("invalid channel %q: must be 'stable' or 'canary'", channel)
			}
			if err := saveChannel(channel); err != nil {
				ui.Warn(fmt.Sprintf("Could not save channel preference: %v", err))
			}
		} else {
			channel = loadChannel()
		}

		method := detectInstallMethod()
		current := version.GetVersion()

		if check {
			ui.Info(fmt.Sprintf("Current version: %s", current))
			ui.Info(fmt.Sprintf("Install method: %s", method))
			ui.Info(fmt.Sprintf("Channel: %s", channel))
			ui.Info(fmt.Sprintf("Platform: %s/%s", runtime.GOOS, runtime.GOARCH))
			return nil
		}

		switch method {
		case "homebrew":
			formula := "nself"
			if channel == "canary" {
				formula = "nself-canary"
			}
			ui.Info(fmt.Sprintf("Upgrading via Homebrew (%s)...", formula))
			ui.Info(fmt.Sprintf("Run: brew upgrade %s", formula))
			return nil

		case "system":
			ui.Warn("nSelf was installed via a system package manager.")
			ui.Info("Use your system package manager to upgrade (e.g. apt upgrade nself, dnf upgrade nself)")
			return nil

		case "direct":
			// Backup current binary before upgrade
			backupPath, err := backupBinary()
			if err != nil {
				ui.Warn(fmt.Sprintf("Could not backup current binary: %v", err))
			} else {
				ui.Dimmed(fmt.Sprintf("Backup saved to %s", backupPath))
			}

			// Delegate to the existing selfUpdate logic from update.go
			tag := ""
			if targetVersion != "" {
				tag = targetVersion
				if !strings.HasPrefix(tag, "v") {
					tag = "v" + tag
				}
			} else {
				latest, _, err := fetchLatestRelease()
				if err != nil {
					return fmt.Errorf("checking for updates: %w", err)
				}
				tag = latest
			}

			if normalizeVersion(current) == normalizeVersion(tag) {
				ui.Success(fmt.Sprintf("Already on latest version (%s)", current))
				return nil
			}

			// Warn about plugins that will be incompatible with the new version.
			pluginDir := resolvePluginDir()
			if incompat := plugin.IncompatiblePlugins(pluginDir, tag); len(incompat) > 0 {
				ui.Warn("The following plugins may be incompatible with " + tag + ":")
				for _, p := range incompat {
					ui.Warn("  - " + p)
				}
				ui.Warn("You may need to update these plugins after upgrading.")
			}

			ui.Info(fmt.Sprintf("Upgrading: %s -> %s", current, tag))
			if err := selfUpdate(tag); err != nil {
				ui.Error(fmt.Sprintf("Upgrade failed: %v", err))
				ui.Info("Run 'nself upgrade --rollback' to restore the previous version")
				return err
			}

			ui.Success(fmt.Sprintf("Upgraded to %s", tag))
			return nil

		default:
			return fmt.Errorf("unknown install method: %s", method)
		}
	},
}

func init() {
	upgradeCmd.Flags().Bool("check", false, "Check for updates without installing")
	upgradeCmd.Flags().String("channel", "", "Release channel: stable or canary")
	upgradeCmd.Flags().String("version", "", "Upgrade to a specific version (e.g. 1.2.3)")
	upgradeCmd.Flags().Bool("rollback", false, "Revert to the previously installed version")
	upgradeCmd.Flags().String("binary-url", "", "Install from a specific binary URL (HTTPS only; .tar.gz/.tgz; operator use)")
	upgradeCmd.Flags().String("binary-sha256", "", "Operator-supplied SHA-256 digest for --binary-url (skips checksums.txt fetch)")
	// CLI-R11 core list: `update` absorbs upgrade. The old top-level spelling
	// keeps working through legacy_spellings.go.
	updateCmd.AddCommand(upgradeCmd)
}
