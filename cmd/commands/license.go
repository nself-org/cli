package commands

import (
	"github.com/spf13/cobra"
)

// defaultPingURL is the production license validation endpoint.
const defaultPingURL = "https://ping.nself.org"

// pricingURL is the page opened by the upgrade subcommand.
const pricingURL = "https://nself.org/pricing"

var licenseCmd = &cobra.Command{
	Use:   "license",
	Short: "Manage license keys for nSelf product bundles",
	Long: `Manage license keys for nSelf product bundles (ɳClaw, ɳChat, nTV, etc.).

Supports multiple keys for different product bundles. Keys can also be set via
environment variables: NSELF_PLUGIN_LICENSE_KEY (legacy) or NSELF_LICENSE_KEY_1
through NSELF_LICENSE_KEY_10.

Subcommands:
  set      Replace all keys with a single key (backward compatible)
  add      Add one or more keys without replacing existing ones
  remove   Remove a key by value or product name
  status   Show all configured keys, products, and plugin coverage
  list     Alias for status
  show     Display saved key (masked) and tier
  validate Validate key against ping.nself.org
  clear    Remove all saved license keys
  upgrade  Open pricing page in browser`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself license` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	// Flags for new commands.
	licenseShowCmd.Flags().Bool("json", false, "Output as JSON")
	licenseHealthCmd.Flags().Bool("json", false, "Output as JSON")
	licenseRevokeCmd.Flags().Bool("yes", false, "Confirm revocation without interactive prompt")
	licenseRestoreCmd.Flags().String("key", "", "New license key to restore with")
	licenseStatusCmd.Flags().String("plugin", "", "Check access for a specific plugin by name")
	licenseListCmd.Flags().String("plugin", "", "Check access for a specific plugin by name")

	licenseCmd.AddCommand(licenseSetCmd)
	licenseCmd.AddCommand(licenseAddCmd)
	licenseCmd.AddCommand(licenseRemoveCmd)
	licenseCmd.AddCommand(licenseStatusCmd)
	licenseCmd.AddCommand(licenseListCmd)
	licenseCmd.AddCommand(licenseShowCmd)
	licenseCmd.AddCommand(licenseValidateCmd)
	licenseCmd.AddCommand(licenseRevalidateCmd)
	licenseCmd.AddCommand(licenseHealthCmd)
	licenseCmd.AddCommand(licenseRevokeCmd)
	licenseCmd.AddCommand(licenseRestoreCmd)
	licenseCmd.AddCommand(licenseClearCmd)
	licenseCmd.AddCommand(licenseUpgradeCmd)
	RootCmd.AddCommand(licenseCmd)
}
