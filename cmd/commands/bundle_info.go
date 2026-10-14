package commands

// Purpose: the "nself bundle info" subcommand, its RunE, and the license/
// plugin-install status helpers it uses (resolveBundleLicenseStatus,
// checkPluginInstalled). Inputs are the cobra command/args or a bundle/plugin
// key; outputs are printed bundle info or a status string/bool. Bundle
// membership is resolved from bundles.json via internal/bundle
// (P6-E4-W3-S3-T10 — no local bundle map here).
// Constraints: split out of bundle.go (CLI-R12) as a pure move, no behavior change.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/plugin"

	"github.com/spf13/cobra"
)

// bundleInfoJSON is the JSON-serialisable form of `nself bundle info --json`.
type bundleInfoJSON struct {
	Slug          string   `json:"slug"`
	Name          string   `json:"name"`
	Price         string   `json:"price"`
	Description   string   `json:"description,omitempty"`
	Plugins       []string `json:"plugins"`
	PluginCount   int      `json:"plugin_count"`
	LicenseStatus string   `json:"license_status"`
	InstallHint   string   `json:"install_hint"`
}

var bundleInfoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show bundle details, plugins, and license status",
	Long: `Display the full plugin membership, pricing, and your current license status for a bundle.

Both short (claw) and legacy n-prefixed (nclaw) slugs are accepted.

Examples:
  nself bundle info claw
  nself bundle info nclaw --json
  nself bundle info nself-plus`,
	Args: cobra.ExactArgs(1),
	RunE: runBundleInfo,
}

func runBundleInfo(cmd *cobra.Command, args []string) error {
	key := strings.ToLower(strings.TrimSpace(args[0]))

	b, ok := bundle.Get(key)
	if !ok {
		return fmt.Errorf("bundle not found: %q\n\nRun 'nself bundle list' for available bundles.\nAvailable: %s", key, strings.Join(bundle.Names(), ", "))
	}
	// Report under the bundle's own canonical slug so both alias forms
	// (nclaw / claw) print identical output, per acceptance criteria.
	slug := b.Slug

	asJSON := false
	if cmd != nil {
		asJSON, _ = cmd.Flags().GetBool("json")
	}

	licenseStatus := resolveBundleLicenseStatus(slug)

	// Build install hint string.
	var installHint string
	switch slug {
	case "task":
		installHint = "nself plugin install <plugin-name>  (no license required)"
	case "nself-plus":
		installHint = "nself license set <key>  |  Buy: https://nself.org/pricing"
	default:
		if len(b.Plugins) > 0 {
			installHint = fmt.Sprintf("nself license set <key> && nself plugin install %s  |  Buy: https://nself.org/pricing", b.Plugins[0])
		} else {
			installHint = "nself license set <key>  |  Buy: https://nself.org/pricing"
		}
	}

	if asJSON {
		plugins := b.Plugins
		if plugins == nil {
			plugins = []string{}
		}
		row := bundleInfoJSON{
			Slug:          slug,
			Name:          b.Name,
			Price:         b.Price,
			Description:   b.Description,
			Plugins:       plugins,
			PluginCount:   len(plugins),
			LicenseStatus: licenseStatus,
			InstallHint:   installHint,
		}
		if jsonEnvelopeOn(cmd) {
			return emitEnv(cmd, row, true) // P7-SURF-13
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(row)
	}

	fmt.Printf("Bundle: %s (%s)\n", b.Name, slug)
	fmt.Printf("Price:  %s\n", b.Price)
	if b.Description != "" {
		fmt.Printf("Note:   %s\n", b.Description)
	}

	// ── License status ─────────────────────────────────────────────
	fmt.Printf("License: %s\n", licenseStatus)

	// ── Plugin membership ──────────────────────────────────────────
	fmt.Println()
	if slug == "nself-plus" {
		fmt.Println("Includes every paid bundle (claw, chat, family, tv, clawde, sentry)")
		fmt.Println("+ all nSelf apps + support via chat.nself.org or the nChat app")
	} else if b.Plugins != nil {
		fmt.Printf("Plugins (%d):\n", len(b.Plugins))
		for _, p := range b.Plugins {
			installed := checkPluginInstalled(p)
			marker := " "
			if installed {
				marker = "✓"
			}
			fmt.Printf("  [%s] %s\n", marker, p)
		}
		fmt.Println()
		fmt.Println("  ✓ = installed on this machine")
	}

	// ── Install hint ───────────────────────────────────────────────
	fmt.Println()
	fmt.Printf("Install: %s\n", installHint)

	return nil
}

// ── Helpers ─────────────────────────────────────────────────────────

// resolveBundleLicenseStatus returns a human-readable license status string
// for the given canonical bundle slug. Reads the local license cache — no
// network call.
func resolveBundleLicenseStatus(bundleSlug string) string {
	if bundleSlug == "task" {
		return "active (free)"
	}

	cache, err := license.ReadCache()
	if err != nil || cache == nil {
		return "not activated (run: nself license set <key>)"
	}

	tier := strings.ToLower(cache.Tier)

	// ɳSelf+ (owner / enterprise / plus tiers) covers all bundles.
	if tier == "plus" || tier == "enterprise" || tier == "owner" {
		return fmt.Sprintf("active via ɳSelf+ (%s tier)", cache.Tier)
	}

	// Check if the specific bundle is in PluginsAllowed.
	// The license server populates PluginsAllowed with all plugin names
	// included in the licensed bundle(s).
	b, ok := bundle.Get(bundleSlug)
	if ok && len(b.Plugins) > 0 {
		for _, allowed := range cache.PluginsAllowed {
			for _, bp := range b.Plugins {
				if strings.EqualFold(allowed, bp) {
					return fmt.Sprintf("active (%s tier)", cache.Tier)
				}
			}
		}
	}

	return fmt.Sprintf("not included in current license (%s tier — run: nself license set <key>)", cache.Tier)
}

// checkPluginInstalled returns true if the plugin appears to be installed locally.
func checkPluginInstalled(pluginName string) bool {
	if strings.HasPrefix(pluginName, "(") {
		// Catalog entries like "(soon)" are display-only, not real plugin names.
		return false
	}
	pluginDir := resolvePluginDir()
	installed, err := plugin.ListInstalled(pluginDir)
	if err != nil {
		return false
	}
	for _, p := range installed {
		if strings.EqualFold(p.Name, pluginName) {
			return true
		}
	}
	return false
}

// ── bundle install ──────────────────────────────────────────────────
