package commands

// Purpose: the bundleListRow type and runBundleList, the RunE for "nself
// bundle list". Inputs are the cobra command/args; outputs are a printed
// table or JSON of canonical bundles resolved from bundles.json via
// internal/bundle (P6-E4-W3-S3-T10 — no local bundle map here).
// Each --json row gains an additive `ledger` object, the row's slice of the
// install-state ledger (internal/bundle/ledger, P7-PLUG-18). The ledger is read
// only here, and only with --json (the table never loads it): when the file does
// not exist yet the view is bootstrapped in memory and nothing is written. A
// damaged ledger file fails `--json` with the file path and the way to
// re-bootstrap it.
// Constraints: split out of bundle.go (CLI-R12) as a pure move, no behavior change.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/plugin"

	"github.com/spf13/cobra"
)

// bundleListRow is the JSON-serialisable form of one bundle in `nself bundle list --json`.
type bundleListRow struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Price       string   `json:"price"`
	PluginCount int      `json:"plugin_count"`
	Plugins     []string `json:"plugins"`
	Status      string   `json:"status"`
	HasActive   bool     `json:"has_active"`
	// Ledger is this bundle's slice of .nself/state/bundles.json (additive).
	Ledger ledger.BundleView `json:"ledger"`
}

// ledgerSources adapts internal/bundle and the installed plugin directory to
// the ledger package's bootstrap input (the ledger imports neither).
func ledgerSources() (ledger.Sources, error) {
	var src ledger.Sources
	for _, b := range bundle.All() {
		src.Bundles = append(src.Bundles, ledger.BundleDef{Slug: b.Slug, Installable: b.IsInstallable(), Plugins: b.Plugins})
	}
	manifests, err := plugin.LoadManifestsFromDir(resolvePluginDir())
	if err != nil {
		return ledger.Sources{}, err
	}
	for _, m := range manifests {
		tier := ledger.TierFree
		if m.Tier == "pro" || m.Tier == "paid" || m.Tier == ledger.TierLicensed || m.RequiresLicense || m.LicenseType == "pro" {
			tier = ledger.TierLicensed
		}
		src.Plugins = append(src.Plugins, ledger.InstalledPlugin{Name: m.Name, Version: m.Version, Tier: tier, Checksum: m.Checksum})
	}
	return src, nil
}

// loadBundleLedger reads the project's ledger without writing it. warn
// receives what bootstrap skipped.
func loadBundleLedger(warn func(string)) (ledger.Ledger, error) {
	root, err := projectRoot()
	if err != nil {
		return ledger.Ledger{}, err
	}
	store := ledger.NewStore(root)
	store.Sources = ledgerSources
	store.Warn = warn
	l, _, err := store.Load()
	return l, err
}

func runBundleList(cmd *cobra.Command, _ []string) error {
	var showInstalled, asJSON bool
	if cmd != nil {
		showInstalled, _ = cmd.Flags().GetBool("installed")
		asJSON, _ = cmd.Flags().GetBool("json")
	}

	// The ledger is read only for --json: the table never shows it, so a
	// damaged or unreadable ledger must not break the plain catalog listing.
	var led ledger.Ledger
	if asJSON {
		var err error
		led, err = loadBundleLedger(func(m string) { _, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning: ledger bootstrap:", m) })
		if err != nil {
			return err
		}
	}

	// Build the row set, optionally filtering to installed-only.
	var rows []bundleListRow
	pluginDir := resolvePluginDir()
	installedPlugins, _ := plugin.ListInstalled(pluginDir)
	installedSet := make(map[string]bool, len(installedPlugins))
	for _, p := range installedPlugins {
		installedSet[strings.ToLower(p.Name)] = true
	}

	for _, b := range bundle.All() {
		status := "active"
		switch b.Slug {
		case "task":
			status = "free"
		}

		// Count how many bundle plugins are installed.
		activeCount := 0
		for _, p := range b.Plugins {
			if installedSet[strings.ToLower(p)] {
				activeCount++
			}
		}
		hasActive := activeCount > 0

		if showInstalled && !hasActive {
			continue
		}

		pluginList := b.Plugins
		if pluginList == nil {
			pluginList = []string{}
		}

		rows = append(rows, bundleListRow{
			Slug:        b.Slug,
			Name:        b.Name,
			Price:       b.Price,
			PluginCount: len(b.Plugins),
			Plugins:     pluginList,
			Status:      status,
			HasActive:   hasActive,
			Ledger:      led.View(b.Slug, b.Plugins),
		})
	}

	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}

	fmt.Println("nSelf Plugin Bundles")
	fmt.Println(strings.Repeat("─", 60))

	for _, r := range rows {
		pluginCount := ""
		if r.PluginCount > 0 {
			pluginCount = fmt.Sprintf("  (%d plugins)", r.PluginCount)
		}

		b, _ := bundle.Get(r.Slug)
		desc := b.Description
		if desc == "" && len(b.Plugins) > 0 {
			preview := b.Plugins
			if len(preview) > 4 {
				preview = append(preview[:4:4], "...")
			}
			desc = strings.Join(preview, ", ")
		}

		fmt.Printf("  %-14s  %-26s  %s%s\n", r.Slug, r.Name, r.Price, pluginCount)
		if desc != "" {
			fmt.Printf("  %-14s  %s\n", "", desc)
		}
		fmt.Println()
	}

	fmt.Println("Run 'nself bundle info <name>' for full plugin membership.")
	fmt.Println("Buy at: https://nself.org/pricing")
	return nil
}

// ── bundle info ─────────────────────────────────────────────────────
