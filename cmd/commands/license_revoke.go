package commands

// Purpose: License revocation/restore commands split out of license.go
// (CLI-R12 Batch B mechanical file-size split). Holds `nself license
// revoke/restore` — the pair of subcommands that move a license between
// active and dormant state.
// Inputs: cobra command flags (--yes, --key).
// Outputs: stdout confirmation messages; errors wrap license/plugin
// failures.
// Constraints: pure move, no behavior change. defaultPingURL const and the
// licenseCmd/init() registration remain in license.go.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/plugin"

	"github.com/spf13/cobra"
)

var licenseRevokeCmd = &cobra.Command{
	Use:   "revoke",
	Short: "Mark local license as revoked and wipe the stored key",
	Long: `Revoke the local license:

  - Marks the local cache as revoked.
  - Wipes the stored license key from disk.
  - Plugins using this license go DORMANT (not uninstalled) on the next build.

Use 'nself license restore --key <new-key>' to reactivate.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		confirm, _ := cmd.Flags().GetBool("yes")
		if !confirm {
			fmt.Println("This will revoke the local license and wipe the stored key.")
			fmt.Println("Plugins will go DORMANT on the next build.")
			fmt.Println("Run with --yes to confirm.")
			setData(cmd, licenseRevokeData{})
			return nil
		}

		if err := license.RevokeLicense(); err != nil {
			return fmt.Errorf("revocation failed: %w", err)
		}

		setData(cmd, licenseRevokeData{Revoked: true})
		fmt.Println("License revoked. Stored key removed.")
		fmt.Println("Plugins will show DORMANT status after next 'nself build'.")
		fmt.Println("To reactivate: nself license restore --key <new-key>")
		return nil
	},
}

var licenseRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Reactivate dormant plugins with a new license key",
	Long: `Restore a previously revoked license:

  1. Validates the new key format.
  2. Validates the key against ping.nself.org.
  3. Clears the revoked marker.
  4. Stores the new key so plugins re-activate on the next build.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		newKey, _ := cmd.Flags().GetString("key")
		newKey = strings.TrimSpace(newKey)
		if newKey == "" {
			return fmt.Errorf("--key <new-key> is required")
		}

		// Format validation.
		if err := license.ValidateKeyFormat(newKey); err != nil {
			return fmt.Errorf("invalid key format: %w", err)
		}

		// Remote validation.
		pingURL := os.Getenv("NSELF_PING_API_URL")
		if pingURL == "" {
			pingURL = defaultPingURL
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		valid, err := plugin.ValidateLicenseRemote(ctx, newKey, pingURL)
		cancel()
		if err != nil {
			return fmt.Errorf("key validation failed: %w", err)
		}
		if !valid {
			return fmt.Errorf("key is invalid or expired on the server")
		}

		// Restore.
		if err := license.RestoreWithKey(newKey); err != nil {
			return fmt.Errorf("restore failed: %w", err)
		}

		pp := license.DetectProduct(newKey)
		setData(cmd, licenseRestoreData{Restored: true, Product: licenseProductName(pp, "")})
		if pp != nil {
			fmt.Printf("%s license restored.\n", pp.DisplayName)
		} else {
			fmt.Println("License restored.")
		}
		fmt.Println("Dormant plugins will re-activate on the next 'nself build'.")
		return nil
	},
}
