package commands

import (
	"context"
	"fmt"

	"github.com/nself-org/cli/internal/bundle"

	"github.com/spf13/cobra"
)

// ── Parent command ──────────────────────────────────────────────────
//
// Bundle membership (name/price/plugins) is resolved from bundles.json via
// internal/bundle — NEVER hand-maintained here. Every subcommand below reads
// through internal/bundle.Get/Names/All so there is exactly one bundle map
// in the CLI (P6-E4-W3-S3-T10; a second hand-copied map here previously
// diverged from internal/bundle's, the same drift shape that caused 68 paid
// plugins to 404 in the license allowlist bug).

var bundleCmd = &cobra.Command{
	Use:   "bundle",
	Short: "Manage and inspect nSelf plugin bundles",
	Long: `Bundle information and management.

Subcommands:
  list         List all available bundles with pricing
  info <name>  Show bundle details, plugins, and license status`,
	// PersistentPreRunE eager-fetches+validates bundles.json once per command
	// invocation (no lazy-init on first Get/All access), backed by cache
	// fallback on fetch failure. See internal/bundle/registry.go.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		return bundle.Load(ctx)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return fmt.Errorf("unknown bundle subcommand %q; run 'nself bundle --help' for the list", args[0])
	},
}

// ── bundle list ─────────────────────────────────────────────────────

var bundleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available nSelf plugin bundles",
	Long: `Display every bundle — name, price, and plugin count.

Paid bundles require a license key set via: nself license set <key>
Buy or manage subscriptions at: https://nself.org/pricing`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBundleList(cmd, args)
	},
}

func init() {
	// list flags
	bundleListCmd.Flags().Bool("installed", false, "Show only bundles with at least one active plugin")
	bundleListCmd.Flags().Bool("json", false, "Output machine-readable JSON array")

	// info flags
	bundleInfoCmd.Flags().Bool("json", false, "Output machine-readable JSON object")

	// install flags
	bundleInstallCmd.Flags().Bool("dry-run", false, "Print the planned actions without installing")
	bundleInstallCmd.Flags().Bool("force", false, "Re-install even if already installed; skips same-version check (repair/upgrade path). License is still validated.")
	bundleInstallCmd.Flags().Bool("strict", false, "Fail if any plugin in the bundle is missing from the registry")
	bundleInstallCmd.Flags().String("channel", "stable", "Release channel: stable | beta | canary")

	// remove flags
	bundleRemoveCmd.Flags().Bool("dry-run", false, "Print the planned actions without removing")
	bundleRemoveCmd.Flags().Bool("delete-data", false, removeDeleteDataHelp)
	bundleRemoveCmd.Flags().Bool("keep-data", false, "Deprecated: no-op from v1.5 (data is kept by default); removed in v1.6.0")
	_ = bundleRemoveCmd.Flags().MarkHidden("keep-data")

	bundleCmd.AddCommand(bundleListCmd)
	bundleCmd.AddCommand(bundleInfoCmd)
	bundleCmd.AddCommand(bundleInstallCmd)
	bundleCmd.AddCommand(bundleRemoveCmd)
	RootCmd.AddCommand(bundleCmd)
}
