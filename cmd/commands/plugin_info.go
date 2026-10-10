package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var pluginInfoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show detailed plugin information",
	Long: `Display detailed information about a plugin from the registry.
Use --open to open the plugin's marketplace page in your browser.

  nself plugin info ai
  nself plugin info ai --open`,
	Args: cobra.ExactArgs(1),
	RunE: runPluginInfo,
}

func init() {
	pluginInfoCmd.Flags().Bool("open", false, "Open the plugin marketplace page in a browser")
	pluginInfoCmd.Flags().Bool("json", false, "Output as JSON")
	pluginCmd.AddCommand(pluginInfoCmd)
}

func runPluginInfo(cmd *cobra.Command, args []string) error {
	name := args[0]
	openBrowser, _ := cmd.Flags().GetBool("open")

	marketplaceURL := fmt.Sprintf("https://plugins.nself.org/%s", name)

	if openBrowser {
		return openURL(marketplaceURL)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	registryURL := os.Getenv("NSELF_PLUGIN_REGISTRY")
	if err := plugin.ValidateNetworkAccess(ctx, registryURL); err != nil {
		return err
	}

	cacheDir := os.Getenv("NSELF_PLUGIN_CACHE")
	if cacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			cacheDir = "/tmp/.nself/cache/plugins"
		} else {
			cacheDir = home + "/.nself/cache/plugins"
		}
	}

	reg, err := plugin.FetchRegistry(ctx, registryURL, cacheDir)
	if err != nil {
		return fmt.Errorf("fetching registry: %w", err)
	}

	pluginDir := resolvePluginDir()
	manifest, tier, tierReason := pluginInfoTier(ctx, reg, name, pluginDir)
	if manifest == nil {
		return fmt.Errorf("plugin %q not found in registry", name)
	}

	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		return writePluginInfoJSON(cmd, manifest, tier, tierReason)
	}

	// Display plugin details.
	tbl := ui.NewTable("Field", "Value")
	tbl.AddRow("Name", manifest.Name)
	tbl.AddRow("Version", manifest.Version)
	tbl.AddRow("Description", manifest.Description)
	tbl.AddRow("Category", manifest.Category)
	tbl.AddRow("License", manifest.License)
	// compat.V15(P7-PLUG-17): the Tier row shows the registry word -> shows the licence word and why (Tier reason)
	if compat.V15() {
		tbl.AddRow("Tier", tier)
		if tierReason != "" {
			tbl.AddRow("Tier reason", tierReason)
		}
	} else {
		tbl.AddRow("Tier", manifest.Tier)
	}

	if manifest.Author != "" {
		tbl.AddRow("Author", manifest.Author)
	}
	if manifest.Language != "" {
		tbl.AddRow("Language", manifest.Language)
	}
	if manifest.Port > 0 {
		tbl.AddRow("Port", fmt.Sprintf("%d", manifest.Port))
	}
	if len(manifest.Dependencies) > 0 {
		tbl.AddRow("Dependencies", strings.Join(manifest.Dependencies, ", "))
	}
	if len(manifest.Tags) > 0 {
		tbl.AddRow("Tags", strings.Join(manifest.Tags, ", "))
	}
	if manifest.Repository != "" {
		tbl.AddRow("Repository", manifest.Repository)
	}
	if manifest.Compat != nil && manifest.Compat.Nself != "" {
		tbl.AddRow("CLI Compat", manifest.Compat.Nself)
	}

	tbl.Render()

	// S71-T03: Display declared permissions (one per line, with risk tier prefix).
	if manifest.Permissions.Len() > 0 {
		fmt.Println("\nPermissions:")
		for _, perm := range manifest.Permissions.Strings() {
			fmt.Printf("  %s %s\n", permissionRiskPrefix(perm), perm)
		}
		if manifest.Permissions.UsesLegacyVocabulary() {
			// The descriptive form IS checked now — reduced to the canonical
			// vocabulary and run through the same allowlist. Showing what it
			// reduced to matters, because the reduction widens: a declaration
			// naming one host is enforced as general internet access.
			fmt.Println("\n  Enforced as:")
			for _, perm := range manifest.Permissions.Effective() {
				fmt.Printf("    %s\n", perm)
			}
		}
		fmt.Println("  (v1.0.9: informational only; v1.1.0 will require explicit confirmation)")
	}

	// Check if installed locally.
	installed, err := plugin.ListInstalled(pluginDir)
	if err == nil {
		for _, p := range installed {
			if strings.EqualFold(p.Name, name) {
				fmt.Printf("\nInstalled: %s (%s)\n", p.Version, p.Status)
				break
			}
		}
	}

	fmt.Printf("\nMarketplace: %s\n", marketplaceURL)
	fmt.Printf("Install:     nself plugin install %s\n", name)

	return nil
}

// pluginInfoTier picks the registry entry to describe and the tier (licence
// vocabulary) and reason to report. An installed plugin reports the tier it is
// installed as, with reason "installed". Otherwise v1.5 describes the entry an
// install would pick; v1.4 keeps the first registry match.
func pluginInfoTier(ctx context.Context, reg *plugin.Registry, name, pluginDir string) (*plugin.PluginManifest, string, string) {
	var first, match *plugin.PluginManifest
	instTier, instReason, installed := plugin.InstalledTier(pluginDir, name)
	for i := range reg.Plugins {
		e := &reg.Plugins[i]
		if !strings.EqualFold(e.Name, name) {
			continue
		}
		if first == nil {
			first = e
		}
		if installed && match == nil && plugin.LicenseValue(e.Tier) == instTier {
			match = e
		}
	}
	if first == nil {
		return nil, "", ""
	}
	// compat.V15(P7-PLUG-17): info describes the first registry match -> describes the installed tier's entry, or the one an install would pick
	if !compat.V15() {
		if installed {
			return first, instTier, instReason
		}
		return first, plugin.LicenseValue(first.Tier), ""
	}
	if installed {
		if match == nil {
			match = first
		}
		return match, instTier, instReason
	}
	if m, reason, err := plugin.ResolvePluginTier(ctx, reg, name, "", "", nil); err == nil {
		return m, plugin.LicenseValue(m.Tier), reason
	}
	return first, plugin.LicenseValue(first.Tier), ""
}

// writePluginInfoJSON prints `plugin info --json`: the registry fields plus
// the reported tier (free or licensed) and why.
func writePluginInfoJSON(cmd *cobra.Command, m *plugin.PluginManifest, tier, reason string) error {
	out := struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Category    string `json:"category"`
		License     string `json:"license"`
		Tier        string `json:"tier"`
		TierReason  string `json:"tier_reason"`
		Port        int    `json:"port,omitempty"`
	}{m.Name, m.Version, m.Description, m.Category, m.License, tier, reason, m.Port}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return err
}

// permissionRiskPrefix returns a short risk label for a permission string.
// Color coding mirrors the admin UI badge classification (S71-T03):
//   - [green]  safe / scoped: db:read, network:plugin:*
//   - [yellow] moderate risk: network:internet, fs:write:*
//   - [red]    high risk: system:exec
//
// Plain-text prefixes are used here (no ANSI) so the output is pipe-safe.
func permissionRiskPrefix(perm string) string {
	switch {
	case perm == "system:exec":
		return "[high]"
	case perm == "network:internet",
		strings.HasPrefix(perm, "fs:write:"):
		return "[med] "
	default:
		return "[low] "
	}
}

// openURL opens the given URL in the default browser.
func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return fmt.Errorf("unsupported platform %q — open %s manually", runtime.GOOS, url)
	}
	return cmd.Start()
}
