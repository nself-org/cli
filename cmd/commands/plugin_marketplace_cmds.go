package commands

// Purpose: `nself plugin marketplace list/search/info` cobra commands and
// RunE handlers, split out of plugin_marketplace.go (CLI-R12 Batch B
// mechanical file-size split). Shared API/render helpers (fetch, filter
// params, table rendering) and the response types remain in
// plugin_marketplace.go.
// Inputs: cobra command flags (--tier, --bundle, --category, --json,
// --open) and positional query/name args.
// Outputs: a rendered table or JSON marketplace listing; for `info --open`,
// opens the plugin's marketplace page in a browser instead.
// Constraints: pure move, no behavior change. pluginCmd registration
// target is defined in plugin.go.

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// --- parent command ---

var pluginMarketplaceCmd = &cobra.Command{
	Use:   "marketplace",
	Short: "Browse the nSelf plugin marketplace",
	Long: `Browse, search, and view details for plugins on the nSelf marketplace.

  nself plugin marketplace list
  nself plugin marketplace list --tier free
  nself plugin marketplace search ai
  nself plugin marketplace info claw`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself plugin marketplace` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// --- list subcommand ---

var pluginMarketplaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all plugins in the marketplace",
	Long: `Fetch and display all plugins available on the nSelf marketplace.

  nself plugin marketplace list
  nself plugin marketplace list --tier free
  nself plugin marketplace list --bundle nclaw
  nself plugin marketplace list --category ai
  nself plugin marketplace list --json`,
	RunE: runPluginMarketplaceList,
}

// --- search subcommand ---

var pluginMarketplaceSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search marketplace plugins by keyword",
	Long: `Search the nSelf marketplace by name, description, or tags.

  nself plugin marketplace search ai
  nself plugin marketplace search "media streaming" --tier pro
  nself plugin marketplace search claw --json`,
	Args: cobra.ExactArgs(1),
	RunE: runPluginMarketplaceSearch,
}

// --- info subcommand ---

var pluginMarketplaceInfoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show detailed marketplace information for a plugin",
	Long: `Display detailed marketplace information for a single plugin.

  nself plugin marketplace info ai
  nself plugin marketplace info claw --json
  nself plugin marketplace info voice --open`,
	Args: cobra.ExactArgs(1),
	RunE: runPluginMarketplaceInfo,
}

func init() {
	// Flags on list.
	pluginMarketplaceListCmd.Flags().String("tier", "", "Filter by tier: free or pro")
	pluginMarketplaceListCmd.Flags().String("bundle", "", "Filter by bundle slug")
	pluginMarketplaceListCmd.Flags().String("category", "", "Filter by category name")
	pluginMarketplaceListCmd.Flags().Bool("json", false, "Output raw JSON")

	// Flags on search.
	pluginMarketplaceSearchCmd.Flags().String("tier", "", "Filter by tier: free or pro")
	pluginMarketplaceSearchCmd.Flags().String("bundle", "", "Filter by bundle slug")
	pluginMarketplaceSearchCmd.Flags().String("category", "", "Filter by category name")
	pluginMarketplaceSearchCmd.Flags().Bool("json", false, "Output raw JSON")

	// Flags on info.
	pluginMarketplaceInfoCmd.Flags().Bool("json", false, "Output raw JSON")
	pluginMarketplaceInfoCmd.Flags().Bool("open", false, "Open the plugin page in a browser")

	// Register subcommands under marketplace.
	pluginMarketplaceCmd.AddCommand(pluginMarketplaceListCmd)
	pluginMarketplaceCmd.AddCommand(pluginMarketplaceSearchCmd)
	pluginMarketplaceCmd.AddCommand(pluginMarketplaceInfoCmd)

	// Register marketplace under plugin.
	pluginCmd.AddCommand(pluginMarketplaceCmd)
}

// --- run functions ---

func runPluginMarketplaceList(cmd *cobra.Command, args []string) error {
	tier, _ := cmd.Flags().GetString("tier")
	bundle, _ := cmd.Flags().GetString("bundle")
	category, _ := cmd.Flags().GetString("category")
	asJSON, _ := cmd.Flags().GetBool("json")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	baseURL := resolveMarketplaceURL()
	params := buildFilterParams(tier, bundle, category, "")

	mr, err := fetchMarketplace(ctx, baseURL, params)
	if err != nil {
		return fmt.Errorf("listing marketplace plugins: %w", err)
	}

	plugins := applyClientFilters(mr.Plugins, tier, bundle, category)

	if len(plugins) == 0 {
		fmt.Fprintln(os.Stderr, "No plugins found matching the given filters.")
		return nil
	}

	if asJSON {
		return ui.PrintJSON(plugins)
	}

	renderMarketplaceTable(plugins)
	fmt.Fprintf(os.Stderr, "\n%d plugin(s) listed.\n", len(plugins))
	return nil
}

func runPluginMarketplaceSearch(cmd *cobra.Command, args []string) error {
	query := args[0]
	tier, _ := cmd.Flags().GetString("tier")
	bundle, _ := cmd.Flags().GetString("bundle")
	category, _ := cmd.Flags().GetString("category")
	asJSON, _ := cmd.Flags().GetBool("json")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	baseURL := resolveMarketplaceURL()
	params := buildFilterParams(tier, bundle, category, query)

	mr, err := fetchMarketplace(ctx, baseURL, params)
	if err != nil {
		return fmt.Errorf("searching marketplace: %w", err)
	}

	plugins := applyClientFilters(mr.Plugins, tier, bundle, category)

	if len(plugins) == 0 {
		fmt.Fprintf(os.Stderr, "No plugins found matching %q.\n", query)
		return nil
	}

	if asJSON {
		return ui.PrintJSON(plugins)
	}

	renderMarketplaceTable(plugins)
	fmt.Fprintf(os.Stderr, "\n%d plugin(s) matched %q.\n", len(plugins), query)
	return nil
}

func runPluginMarketplaceInfo(cmd *cobra.Command, args []string) error {
	name := args[0]
	asJSON, _ := cmd.Flags().GetBool("json")
	openBrowser, _ := cmd.Flags().GetBool("open")

	baseURL := resolveMarketplaceURL()
	pluginPageURL := baseURL + "/" + name

	if openBrowser {
		return openURL(pluginPageURL)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mr, err := fetchMarketplace(ctx, baseURL, url.Values{})
	if err != nil {
		return fmt.Errorf("fetching marketplace for plugin info: %w", err)
	}

	var found *marketplacePlugin
	for i := range mr.Plugins {
		if strings.EqualFold(mr.Plugins[i].Name, name) {
			found = &mr.Plugins[i]
			break
		}
	}
	if found == nil {
		return fmt.Errorf("plugin %q not found in marketplace", name)
	}

	if asJSON {
		return ui.PrintJSON(found)
	}

	tbl := ui.NewTable("Field", "Value")
	tbl.AddRow("Name", found.Name)
	tbl.AddRow("Display Name", found.DisplayName)
	tbl.AddRow("Version", found.Version)
	tbl.AddRow("Description", found.Description)
	tbl.AddRow("Category", found.Category)
	tbl.AddRow("Tier", found.Tier)
	tbl.AddRow("Bundle", found.Bundle)
	if found.Author != "" {
		tbl.AddRow("Author", found.Author)
	}
	tbl.AddRow("Price", found.Price)
	if found.Rating > 0 {
		tbl.AddRow("Rating", fmt.Sprintf("%.1f (%d reviews)", found.Rating, found.ReviewCount))
	}
	if found.Downloads > 0 {
		tbl.AddRow("Downloads", fmt.Sprintf("%d", found.Downloads))
	}
	if len(found.Tags) > 0 {
		tbl.AddRow("Tags", strings.Join(found.Tags, ", "))
	}
	if len(found.Related) > 0 {
		tbl.AddRow("Related", strings.Join(found.Related, ", "))
	}
	if found.Homepage != "" {
		tbl.AddRow("Homepage", found.Homepage)
	}
	if found.Repository != "" {
		tbl.AddRow("Repository", found.Repository)
	}
	tbl.Render()

	fmt.Printf("\nMarketplace: %s\n", pluginPageURL)
	fmt.Printf("Install:     nself plugin install %s\n", found.Name)
	return nil
}
