package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
)

var pluginSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search plugins by name, description, or tag",
	Long: `Search the plugin registry by keyword. Matches against name, description,
category, and tags.

Examples:
  nself plugin search ai
  nself plugin search notification
  nself plugin search --free only
  nself plugin search video --json`,
	Args: cobra.MinimumNArgs(1),
	RunE: runPluginSearch,
}

func init() {
	pluginSearchCmd.Flags().Bool("free", false, "Show only free (MIT) plugins")
	pluginSearchCmd.Flags().Bool("licensed", false, "Show only Licensed plugins")
	pluginSearchCmd.Flags().Bool("pro", false, "Deprecated alias for --licensed")
	_ = pluginSearchCmd.Flags().MarkHidden("pro")
	pluginSearchCmd.Flags().Bool("json", false, "Output as JSON")
	pluginCmd.AddCommand(pluginSearchCmd)
}

var proSearchWarning sync.Once

func runPluginSearch(cmd *cobra.Command, args []string) error {
	query := strings.ToLower(strings.Join(args, " "))
	freeOnly, _ := cmd.Flags().GetBool("free")
	proOnly, _ := cmd.Flags().GetBool("pro")
	licensedOnly, _ := cmd.Flags().GetBool("licensed")
	if proOnly {
		proSearchWarning.Do(func() {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "--pro is deprecated; use --licensed (removal v1.6.0)")
		})
	}
	proOnly = proOnly || licensedOnly
	jsonOut, _ := cmd.Flags().GetBool("json")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	registryURL := os.Getenv("NSELF_PLUGIN_REGISTRY")

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
		return fmt.Errorf("fetching plugin registry: %w", err)
	}

	// Filter by query and optional tier.
	var results []plugin.PluginManifest
	for _, p := range reg.Plugins {
		if freeOnly && p.Tier != "free" {
			continue
		}
		if proOnly && p.Tier == "free" {
			continue
		}
		if matchesQuery(p, query) {
			results = append(results, p)
		}
	}

	if len(results) == 0 {
		fmt.Printf("  %s No plugins matched %q.\n", ui.C(ui.Yellow, ui.IconWarning), query)
		fmt.Printf("  Try: %s to see all plugins.\n", ui.C(ui.Cyan, "nself plugin list"))
		return nil
	}

	if jsonOut {
		return printPluginSearchJSON(results)
	}

	tbl := ui.NewTable("Name", "License", "Category", "Description")
	for _, p := range results {
		tier := p.Tier
		if tier == "free" {
			tier = ui.C(ui.Green, plugin.Label(tier))
		} else {
			tier = ui.C(ui.Yellow, plugin.Label(tier))
		}
		desc := p.Description
		if len(desc) > 55 {
			desc = desc[:52] + "..."
		}
		tbl.AddRow(p.Name, tier, p.Category, desc)
	}
	tbl.Render()

	fmt.Printf("\n  %d result(s) for %q\n", len(results), query)
	fmt.Printf("  Details: %s\n", ui.C(ui.Cyan, "nself plugin info <name>"))
	fmt.Printf("  Install:  %s\n", ui.C(ui.Cyan, "nself plugin install <name>"))
	fmt.Println()
	return nil
}

// printPluginPostInstallHint prints environment-aware hints after a plugin is
// installed. Hints are plugin-specific where known, generic otherwise.
func printPluginPostInstallHint(name string) {
	hints := map[string]string{
		"ai":               "Set your AI provider key: OPENAI_API_KEY or ANTHROPIC_API_KEY in .env.secrets",
		"mux":              "Configure MUX_API_KEY in .env.secrets, then rebuild: nself build && nself start",
		"claw":             "Start ɳClaw backend: nself build && nself start — then open the nClaw app",
		"notify":           "Configure your push provider keys (FCM/APNS) in .env.secrets",
		"livekit":          "Set LIVEKIT_API_KEY and LIVEKIT_API_SECRET in .env.secrets",
		"billing":          "Set STRIPE_SECRET_KEY and STRIPE_WEBHOOK_SECRET in .env.secrets",
		"media-processing": "Rebuild to include the media container: nself build && nself start",
		"sync":             "Enable real-time sync in your app: rebuild after updating .env",
		"realtime":         "Rebuild to activate the realtime plugin: nself build && nself start",
		"recording":        "Set RECORDING_STORAGE_PATH in .env to configure output directory",
	}

	hint, ok := hints[name]
	if !ok {
		hint = "Rebuild to activate: nself build && nself start"
	}
	fmt.Printf("  %s %s\n", ui.C(ui.Blue, ui.IconArrow), hint)
}

// matchesQuery returns true if any searchable field of the manifest contains
// the query string (case-insensitive).
func matchesQuery(p plugin.PluginManifest, query string) bool {
	fields := []string{
		strings.ToLower(p.Name),
		strings.ToLower(p.Description),
		strings.ToLower(p.Category),
	}
	for _, tag := range p.Tags {
		fields = append(fields, strings.ToLower(tag))
	}
	for _, f := range fields {
		if strings.Contains(f, query) {
			return true
		}
	}
	return false
}

// printPluginSearchJSON renders search results as newline-delimited JSON objects.
func printPluginSearchJSON(results []plugin.PluginManifest) error {
	for _, p := range results {
		// compat.V15(P7-PLUG-11): tier pro -> license licensed
		if compat.V15() {
			fmt.Printf(`{"name":%q,"license":%q,"category":%q,"description":%q}`+"\n",
				p.Name, plugin.LicenseValue(p.Tier), p.Category, p.Description)
			continue
		}
		fmt.Printf(`{"name":%q,"tier":%q,"category":%q,"description":%q}`+"\n",
			p.Name, p.Tier, p.Category, p.Description)
	}
	return nil
}
