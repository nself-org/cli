package commands

// Purpose: Read-only plugin query commands split out of plugin.go (CLI-R12
// Batch B mechanical file-size split). Holds `nself plugin list`, `nself
// plugin inventory`, and `nself plugin status` — the three RunE handlers that
// only report on plugin state without mutating it.
// Inputs: cobra command flags (--installed, --detailed, --category,
// --show-eol) and positional plugin name args, plus the shared
// resolvePluginDir()/plugin package helpers defined in plugin.go.
// Outputs: formatted stdout listings/tables; errors wrap the underlying
// plugin package failures.
// Constraints: pure move, no behavior change — cobra.Command var
// declarations and RunE wiring remain in plugin.go.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

func runPluginList(cmd *cobra.Command, args []string) error {
	installed, _ := cmd.Flags().GetBool("installed")
	detailed, _ := cmd.Flags().GetBool("detailed")
	category, _ := cmd.Flags().GetString("category")
	showEOL, _ := cmd.Flags().GetBool("show-eol")    // S58-T03
	available, _ := cmd.Flags().GetBool("available") // OWNER-ACTIONS.md item 15

	if available {
		return runPluginListAvailable(cmd, category)
	}

	pluginDir := resolvePluginDir()

	plugins, err := plugin.List(pluginDir, installed)
	if err != nil {
		return fmt.Errorf("listing plugins: %w", err)
	}

	if len(plugins) == 0 {
		if installed {
			fmt.Println("No plugins installed.")
		} else {
			fmt.Println("No plugins found in registry.")
		}
		return nil
	}

	// CLI-R16: the registry has no per-plugin freshness field today (only a
	// registry-wide snapshot timestamp), so surface that instead of inventing
	// a per-row value. Only meaningful for the registry view, not --installed.
	if detailed && !installed {
		if fetchedAt := plugin.RegistryFetchedAt(); fetchedAt != "" {
			fmt.Printf("Registry snapshot: %s\n\n", fetchedAt)
		}
	}

	for _, p := range plugins {
		// S58-T03: EOL plugins are hidden from listing unless --show-eol is set.
		if p.PublishStatus == "eol" && !showEOL {
			continue
		}
		if category != "" && !strings.EqualFold(p.Category, category) {
			continue
		}
		stateTag := ""
		if p.Running {
			stateTag = " [running]"
		} else if p.Installed {
			stateTag = " [installed]"
		}
		// S58-T03: Full status badge for all 6 lifecycle values.
		statusBadge := ""
		switch p.PublishStatus {
		case "experimental":
			statusBadge = " [experimental]"
		case "planned":
			statusBadge = " [planned]"
		case "beta":
			statusBadge = " [beta]"
		case "deprecated":
			statusBadge = " [DEPRECATED]"
		case "eol":
			statusBadge = " [EOL]"
			// "stable" and "" show no badge — clean install signal
		}
		if detailed {
			// CLI-R16: show per-plugin UpdatedAt only when the source (registry
			// entry or local plugin.json) actually provided one. "-" signals
			// "unknown", never a fabricated date.
			updated := p.UpdatedAt
			if updated == "" {
				updated = "-"
			}
			fmt.Printf("%-20s %-10s %-12s %-12s%s%s\n", p.Name, p.Version, p.Category, updated, stateTag, statusBadge)
		} else {
			fmt.Printf("%-20s%s%s\n", p.Name, stateTag, statusBadge)
		}
	}

	return nil
}

// runPluginListAvailable implements `nself plugin list --available`
// (OWNER-ACTIONS.md item 15): every registry entry for a slug served more
// than once (a declared free/pro tier pair or, for anything else, a
// registry-data collision `nself plugin install` refuses to resolve), with
// the entitlement-resolved default marked so an operator can see the split
// before installing.
func runPluginListAvailable(cmd *cobra.Command, category string) error {
	rows, err := plugin.ListAvailableWithTiers(cmd.Context(), nil)
	if err != nil {
		return fmt.Errorf("listing available plugin tiers: %w", err)
	}
	if len(rows) == 0 {
		fmt.Println("No plugins found in registry.")
		return nil
	}
	if category != "" {
		filtered := rows[:0]
		for _, r := range rows {
			if strings.EqualFold(r.Category, category) {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}
	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		result := make([]map[string]interface{}, 0, len(rows))
		for _, r := range rows {
			item := map[string]interface{}{"name": r.Name, "version": r.Version, "category": r.Category, "tier_pair": r.TierPair, "is_default": r.IsDefault}
			// compat.V15(P7-PLUG-11): tier pro -> license licensed
			if compat.V15() {
				item["license"] = plugin.LicenseValue(r.Tier)
			} else {
				item["tier"] = r.Tier
			}
			result = append(result, item)
		}
		return enc.Encode(result)
	}

	tbl := ui.NewTable("Name", "License", "Version", "Default")
	for _, r := range rows {
		def := ""
		if r.IsDefault {
			def = "✓"
		}
		tier := plugin.Label(r.Tier)
		if tier == "" {
			tier = "-"
		}
		tbl.AddRow(r.Name, tier, r.Version, def)
	}
	tbl.Render()
	fmt.Println("\nDefault = what a plain 'nself plugin install <name>' resolves to today (license entitlement for a tier pair, otherwise its only entry). Override with --tier free|licensed.")
	return nil
}

func runPluginInventory(cmd *cobra.Command, args []string) error {
	pluginDir := resolvePluginDir()

	plugins, err := plugin.ListInstalled(pluginDir)
	if err != nil {
		return fmt.Errorf("listing installed plugins: %w", err)
	}

	if len(plugins) == 0 {
		fmt.Println("No plugins installed. Run `nself plugin install <name>` to get started.")
		return nil
	}

	tbl := ui.NewTable("Name", "Version", "License", "Status", "Description")
	for _, p := range plugins {
		tbl.AddRow(p.Name, p.Version, plugin.Label(p.Tier), p.Status, p.Description)
	}
	tbl.Render()

	return nil
}

func runPluginStatus(cmd *cobra.Command, args []string) error {
	detailed, _ := cmd.Flags().GetBool("detailed")

	if len(args) == 1 {
		name := args[0]
		st, err := plugin.Status(name)
		if err != nil {
			return fmt.Errorf("getting status for %q: %w", name, err)
		}
		if detailed {
			fmt.Printf("Plugin:  %s\nState:   %s\nPID:     %d\n", st.Name, st.State, st.PID)
		} else {
			fmt.Printf("%-20s %s\n", st.Name, st.State)
		}
		return nil
	}

	// No name given: show status for all installed plugins.
	pluginDir := resolvePluginDir()
	plugins, err := plugin.List(pluginDir, true)
	if err != nil {
		return fmt.Errorf("listing installed plugins: %w", err)
	}
	if len(plugins) == 0 {
		fmt.Println("No plugins installed.")
		return nil
	}

	for _, p := range plugins {
		st, err := plugin.Status(p.Name)
		if err != nil {
			fmt.Printf("%-20s unknown\n", p.Name)
			continue
		}
		if detailed {
			fmt.Printf("%-20s %-10s pid=%d\n", st.Name, st.State, st.PID)
		} else {
			fmt.Printf("%-20s %s\n", st.Name, st.State)
		}
	}

	return nil
}
