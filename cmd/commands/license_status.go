package commands

// Purpose: Read-only license reporting commands split out of license.go
// (CLI-R12 Batch B mechanical file-size split). Holds `nself license
// status/list/show` — the RunE handlers (and their shared helper
// timeFromUnix) that only report on configured keys without mutating them.
// Inputs: cobra command flags (--plugin, --json) and the keys returned by
// license.CollectLicenseKeys().
// Outputs: a formatted table or JSON document; errors wrap license/plugin
// package failures.
// Constraints: pure move, no behavior change. defaultPingURL/pricingURL
// consts and the licenseCmd/init() registration remain in license.go.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var licenseStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show all configured licenses and plugin coverage",
	Long: `Show all configured license keys, their status, and plugin coverage.

Use --plugin <name> to check whether a specific plugin is accessible with the
current license. Rate limiting is surfaced as a distinct status line so it is
never confused with a network outage or invalid key.`,
	RunE: runLicenseStatus,
}

var licenseListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show all configured licenses (alias for status)",
	RunE:  runLicenseStatus,
}

func runLicenseStatus(cmd *cobra.Command, args []string) error {
	pluginFilter, _ := cmd.Flags().GetString("plugin")

	keys := license.CollectLicenseKeys()
	doc := newLicenseStatusData()
	if len(keys) == 0 {
		setData(cmd, doc)
		fmt.Println("No license keys configured.")
		fmt.Printf("\nGet a product license at %s\n", pricingURL)
		return nil
	}

	pingURL := os.Getenv("NSELF_PING_API_URL")
	if pingURL == "" {
		pingURL = defaultPingURL
	}

	tbl := ui.NewTable("License", "Product", "Status", "Expires", "Plugins")

	var allProducts []string
	allPlugins := make(map[string]bool)
	hasPlus := false

	for _, key := range keys {
		masked := license.MaskKey(key)
		pp := license.DetectProduct(key)
		productName := "Unknown"
		if pp != nil {
			productName = pp.DisplayName
			if pp.Product == "plus" || pp.Product == "owner" {
				hasPlus = true
			}
		}

		// Try to validate and get entitlements.
		ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
		valid, lvr, err := plugin.ValidateLicenseRemoteWithDetails(ctx, key, pingURL)
		cancel()

		status := "Unknown"
		expires := "-"
		plugins := "-"
		var rowPlugins []string

		if retryAfter, isRL := plugin.IsRateLimited(err); isRL {
			status = fmt.Sprintf("RATE LIMITED — retry after %ss", retryAfter)
			err = nil // already handled; fall through to row render
		}

		if err != nil {
			status = "Error"
		} else if status == "Unknown" && !valid {
			status = "Invalid"
		} else if status == "Unknown" {
			status = "Active"
			if lvr != nil {
				if lvr.Tier != "" {
					productName = lvr.Tier
				}
				if lvr.Expires != "" {
					expires = lvr.Expires
				}
				if len(lvr.Plugins) > 0 {
					rowPlugins = lvr.Plugins
					for _, p := range lvr.Plugins {
						allPlugins[p] = true
					}
					if len(lvr.Plugins) > 5 {
						plugins = strings.Join(lvr.Plugins[:5], ", ") + fmt.Sprintf(" (+%d more)", len(lvr.Plugins)-5)
					} else {
						plugins = strings.Join(lvr.Plugins, ", ")
					}
				}
			}
		}

		allProducts = append(allProducts, productName)
		doc.Keys = append(doc.Keys, licenseStatusKey{Key: masked, Product: licenseScrub(productName), Status: status, Expires: licenseScrubShape(expires), Plugins: licenseScrubAll(rowPlugins)})
		tbl.AddRow(masked, productName, status, expires, plugins)
	}

	tbl.Render()
	doc.Products = licenseScrubAll(uniqueStrings(allProducts))
	for p := range allPlugins {
		doc.Plugins = append(doc.Plugins, licenseScrub(p))
	}
	sort.Strings(doc.Plugins)
	if pluginFilter != "" {
		doc.PluginAccess = &licensePluginAccess{Plugin: pluginFilter, Granted: allPlugins[pluginFilter]}
	}
	setData(cmd, doc)

	// Summary.
	if len(allProducts) > 0 {
		fmt.Printf("\nProducts: %s\n", strings.Join(uniqueStrings(allProducts), ", "))
	}
	if len(allPlugins) > 0 {
		var pluginList []string
		for p := range allPlugins {
			pluginList = append(pluginList, p)
		}
		fmt.Printf("Plugins available: %s\n", strings.Join(pluginList, ", "))
	}

	if !hasPlus {
		fmt.Printf("\nUpgrade to ɳSelf+ ($3.99/mo or $39.99/yr) for all plugins: %s\n", pricingURL)
	}

	// Per-plugin access check (--plugin flag).
	if pluginFilter != "" {
		fmt.Printf("\nPlugin access check: %s\n", pluginFilter)
		if allPlugins[pluginFilter] {
			fmt.Printf("  Access: granted (covered by active license)\n")
		} else {
			fmt.Printf("  Access: denied (not in any active license)\n")
			fmt.Printf("  Get access: nself.org/bundles\n")
		}
	}

	return nil
}

// licenseShowJSON is the structured output for --json mode of `nself license show`.
type licenseShowJSON struct {
	KeyPrefix    string   `json:"key_prefix"`
	Tier         string   `json:"tier"`
	Bundles      []string `json:"bundles_unlocked"`
	PluginsCount int      `json:"plugins_available_count"`
	DaysToExpire *int     `json:"days_to_expire,omitempty"`
	CacheExpiry  string   `json:"cache_expiry,omitempty"`
}

var licenseShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display license tier, bundles, plugins, and cache expiry",
	Long: `Display the current license configuration.

Shows tier, bundles unlocked, plugins available, days until the server-side
license expires, and cache TTL status. The raw key is never shown — only the
last 4 characters are printed as a prefix hint.

Use --json for machine-readable output.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonMode, _ := cmd.Flags().GetBool("json")

		keys := license.CollectLicenseKeys()
		if len(keys) == 0 {
			if jsonMode {
				if jsonEnvelopeOn(cmd) {
					return emitLicenseShow(cmd, nil)
				}
				return json.NewEncoder(os.Stdout).Encode(map[string]string{"error": "no license key configured"})
			}
			fmt.Println("No license key configured.")
			fmt.Printf("Get a license at %s\n", pricingURL)
			return nil
		}

		// Use first key for display (status shows all keys).
		key := keys[0]
		masked := license.MaskKey(key)
		tier := license.DetectTierFromKey(key)
		pp := license.DetectProduct(key)

		// Last 4 chars of key as prefix hint (per S132-T04 spec).
		keyHint := key
		if len(key) > 4 {
			keyHint = "..." + key[len(key)-4:]
		}

		// Read cache for TTL/expiry info.
		cache, _ := license.ReadCache()
		var daysToExpire *int
		cacheExpiry := ""
		if cache != nil {
			fetchedAt := cache.FetchedAt
			ttlExpiry := license.CacheTTLExpiry(cache.Tier, timeFromUnix(fetchedAt))
			remaining := time.Until(ttlExpiry)
			if remaining > 0 {
				days := int(remaining.Hours() / 24)
				daysToExpire = &days
				cacheExpiry = ttlExpiry.Format("2006-01-02")
			} else {
				zero := 0
				daysToExpire = &zero
				cacheExpiry = "expired"
			}
		}

		// Bundles unlocked — computed from bundles.json (ADR-P6-03) as the
		// union of every paid bundle this key entitles, never a single
		// hand-picked product name. See resolveBundlesUnlocked's doc
		// comment for the plus/single-bundle/degrade-on-load-failure rules.
		bundles := resolveBundlesUnlocked(cmd.Context(), pp)

		// Plugins from cache.
		var pluginsCount int
		if cache != nil {
			pluginsCount = len(cache.PluginsAllowed)
		}

		if jsonMode {
			out := licenseShowJSON{
				KeyPrefix:    keyHint,
				Tier:         tier,
				Bundles:      bundles,
				PluginsCount: pluginsCount,
				DaysToExpire: daysToExpire,
				CacheExpiry:  cacheExpiry,
			}
			if jsonEnvelopeOn(cmd) {
				return emitLicenseShow(cmd, &out)
			}
			return json.NewEncoder(os.Stdout).Encode(out)
		}

		fmt.Printf("Key:              %s\n", masked)
		fmt.Printf("Key hint:         %s\n", keyHint)
		fmt.Printf("Tier:             %s\n", tier)
		if len(bundles) > 0 {
			fmt.Printf("Bundles unlocked: %s\n", strings.Join(bundles, ", "))
		}
		if pluginsCount > 0 {
			fmt.Printf("Plugins available: %d\n", pluginsCount)
		}
		if daysToExpire != nil {
			if *daysToExpire == 0 || cacheExpiry == "expired" {
				fmt.Println("Cache expiry:     expired — run 'nself license revalidate' after connecting")
			} else {
				fmt.Printf("Cache expires:    %s (%d day(s))\n", cacheExpiry, *daysToExpire)
			}
		} else {
			fmt.Println("Cache expiry:     no cache — run 'nself license validate' to populate")
		}

		// Pre-expiry warning (S132-T01 / T02).
		if cache != nil {
			if msg, _ := license.PreExpiryWarning(cache); msg != "" {
				fmt.Fprintf(os.Stderr, "Warning: %s\n", msg)
			}
		}

		return nil
	},
}

// timeFromUnix converts a Unix timestamp (int64) to time.Time.
func timeFromUnix(ts int64) time.Time {
	return time.Unix(ts, 0)
}
