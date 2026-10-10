package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/plugin"

	"github.com/spf13/cobra"
)

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage nSelf plugins",
	Long: `Plugin lifecycle management: list, install, remove, update, start, stop, and status.

Unknown subcommands are proxied to the matching plugin binary:
  nself plugin <name> <action>  →  nself-<name> <action>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// If args are present and don't match a subcommand, proxy to the plugin.
		if len(args) >= 1 {
			pluginName := args[0]
			pluginArgs := args[1:]
			return plugin.ProxyCommand(pluginName, pluginArgs)
		}
		return cmd.Help()
	},
}

// --- subcommands ---

var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available and installed plugins",
	RunE:  runPluginList,
}

var pluginInstallCmd = &cobra.Command{
	Use:   "install <plugin> [plugin...]",
	Short: "Install one or more plugins (license check for Licensed plugins); a plugin arg may be a name or an https:// URL",
	Long: `Install one or more plugins.

Official by name, third-party by URL:
  nself plugin install ai            official registry plugin — license
                                      checked for Licensed plugins, Ed25519 signature
                                      verified against a registry-pinned key.
  nself plugin install https://...   third-party source — never touches the
                                      registry, no signature verification.
                                      Pass --checksum <sha256> to verify the
                                      download against a value you obtained
                                      out-of-band (e.g. the plugin's README);
                                      without it, integrity is unverified.
                                      Requires interactive confirmation
                                      naming the source host unless --yes is
                                      passed (for CI).`,
	Args: cobra.MinimumNArgs(1),
	RunE: runPluginInstall,
}

var pluginRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a plugin",
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginRemove,
}

var pluginUpdateCmd = &cobra.Command{
	Use:   "update [name]",
	Short: "Update a specific plugin or all plugins",
	Long: `Update one plugin, or every installed plugin.

An installed plugin keeps its tier (free or licensed) when it updates; if it
cannot (the licence lapsed, or the registry serves only the other tier) the
update stops with E131. Pass --tier with a plugin name to switch on purpose.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPluginUpdateTier,
}

var pluginUpdatesCmd = &cobra.Command{
	Use:   "updates",
	Short: "Check for available plugin updates",
	RunE:  runPluginUpdates,
}

var pluginRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Force refresh the registry cache",
	RunE:  runPluginRefresh,
}

var pluginStartCmd = &cobra.Command{
	Use:   "start <name>",
	Short: "Start a plugin service",
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginStart,
}

var pluginStopCmd = &cobra.Command{
	Use:   "stop <name>",
	Short: "Stop a plugin service",
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginStop,
}

var pluginDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable a plugin (excluded from compose on next build)",
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginDisable,
}

var pluginEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Re-enable a previously disabled plugin",
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginEnable,
}

var pluginStatusCmd = &cobra.Command{
	Use:   "status [name]",
	Short: "Show plugin status",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runPluginStatus,
}

var pluginInventoryCmd = &cobra.Command{
	Use:   "inventory",
	Short: "List installed plugins with version, tier, and status",
	RunE:  runPluginInventory,
}

func init() {
	// Flags on the list subcommand.
	pluginListCmd.Flags().Bool("installed", false, "Show only installed plugins")
	pluginListCmd.Flags().Bool("detailed", false, "Show detailed information")
	pluginListCmd.Flags().String("category", "", "Filter by category")
	pluginListCmd.Flags().Bool("show-eol", false, "Include EOL plugins in listing (hidden by default)") // S58-T03
	pluginListCmd.Flags().Bool("available", false, "List every registry tier of a slug served twice (free+pro pairs), marking the resolved default")
	pluginListCmd.Flags().Bool("json", false, "Output as JSON")

	// Flags on install.
	pluginInstallCmd.Flags().String("key", "", "License key for Licensed plugins")
	pluginInstallCmd.Flags().String("tier", "", `Force "free" or "licensed" for a slug served as both (e.g. cron, notify); default resolves by license entitlement`)
	pluginInstallCmd.Flags().String("version", "", "Install a specific version")
	pluginInstallCmd.Flags().Bool("force", false, "Required when using NSELF_LICENSE_SKIP_VERIFY; explicit acknowledgment of skipped validation")
	pluginInstallCmd.Flags().Bool("allow-eol", false, "Allow installing an EOL plugin (not recommended)") // S58-T03
	pluginInstallCmd.Flags().Bool("preview", false, "Preview the dependency tree without installing")
	pluginInstallCmd.Flags().Bool("with-optional", false, "Include optional dependencies in --preview output")
	pluginInstallCmd.Flags().Bool("skip-sbom-check", false, "Skip SBOM verification (air-gapped installs only — sets NSELF_SKIP_SBOM_CHECK=1)") // S2.T12
	pluginInstallCmd.Flags().Bool("dry-run", false, "Show what would be installed without making changes")
	pluginInstallCmd.Flags().Bool("show-graph", false, "Show dependency graph with topological sort order")
	pluginInstallCmd.Flags().Bool("yes", false, "Skip the third-party source confirmation prompt (non-interactive/CI use)")
	pluginInstallCmd.Flags().String("checksum", "", "Expected SHA-256 checksum of the downloaded archive (third-party URL installs only)")

	// Flags on update.
	pluginUpdateCmd.Flags().Bool("allow-eol", false, "Allow updating to/from an EOL plugin (not recommended)") // S58-T03
	pluginUpdateCmd.Flags().String("tier", "", `Switch the plugin to "free" or "licensed" on purpose (needs a plugin name); default keeps the installed tier`)

	// Flags on remove.
	pluginRemoveCmd.Flags().Bool("keep-data", false, "Preserve database data on remove")
	pluginRemoveCmd.Flags().Bool("force", false, "Remove even if other plugins depend on this one")

	// Flags on status.
	pluginStatusCmd.Flags().Bool("detailed", false, "Show detailed status")

	// Register subcommands.
	pluginCmd.AddCommand(pluginListCmd)
	pluginCmd.AddCommand(pluginInstallCmd)
	pluginCmd.AddCommand(pluginRemoveCmd)
	pluginCmd.AddCommand(pluginUpdateCmd)
	pluginCmd.AddCommand(pluginUpdatesCmd)
	pluginCmd.AddCommand(pluginRefreshCmd)
	pluginCmd.AddCommand(pluginStartCmd)
	pluginCmd.AddCommand(pluginStopCmd)
	pluginCmd.AddCommand(pluginDisableCmd)
	pluginCmd.AddCommand(pluginEnableCmd)
	pluginCmd.AddCommand(pluginStatusCmd)
	pluginCmd.AddCommand(pluginInventoryCmd)

	RootCmd.AddCommand(pluginCmd)
}

// --- helpers ---

// resolvePluginDir returns the plugin directory from config or a sensible default.
func resolvePluginDir() string {
	if d := os.Getenv("NSELF_PLUGIN_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/tmp", ".nself", "plugins")
	}
	return filepath.Join(home, ".nself", "plugins")
}

// loadConfig loads the project config from the current working directory.
func loadConfig() (*config.Config, error) {
	workdir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}
	cfg, err := config.Load(workdir)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	return cfg, nil
}

// runPluginUpdateTier is the RunE of `plugin update`. Without --tier it is
// runPluginUpdate; with --tier (one plugin) the tier goes to Update as a
// parameter, so the switch is explicit and nothing else carries it.
func runPluginUpdateTier(cmd *cobra.Command, args []string) error {
	tier, _ := cmd.Flags().GetString("tier")
	tier = strings.ToLower(strings.TrimSpace(tier))
	if tier == "" {
		return runPluginUpdate(cmd, args)
	}
	if tier != "free" && tier != plugin.LicenseLicensed && tier != plugin.WireTierPro {
		return fmt.Errorf(`invalid --tier %q: must be "free" or "licensed"`, tier)
	}
	if len(args) != 1 {
		return fmt.Errorf("--tier needs a plugin name: nself plugin update <name> --tier %s", tier)
	}
	name := args[0]
	ctx := context.Background()
	if err := plugin.ValidateNetworkAccess(ctx, os.Getenv("NSELF_PLUGIN_REGISTRY")); err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Updating plugin %q (tier %s)...\n", name, tier)
	if err := plugin.UpdateWithTier(ctx, cfg, name, resolvePluginDir(), tier); err != nil {
		return fmt.Errorf("updating plugin %q: %w", name, err)
	}
	fmt.Fprintf(os.Stderr, "Plugin %q updated successfully.\n", name)
	return nil
}
