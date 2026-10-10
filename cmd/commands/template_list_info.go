package commands

// Purpose: Implements `nself template list` and `nself template info
// <slug>`, plus the manifest-validation helper shared with publish. Split
// out of template.go (CLI-R12) to separate these read-path handlers from
// the cobra command wiring (template.go), the registry HTTP client
// (template_registry.go), and the write-path handlers
// (template_publish_update.go).
// Inputs: the cobra.Command + args/flags for list (category/search
// filters) and info (a template slug).
// Outputs: a printed template table or single-template detail view.
// Constraints: pure move — no behavior changes. validateTemplateManifest
// is also called from runTemplatePublish in template_publish_update.go.

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/templates/clone"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

func runTemplateList(cmd *cobra.Command, args []string) error {
	category, _ := cmd.Flags().GetString("category")
	freeOnly, _ := cmd.Flags().GetBool("free")
	sortBy, _ := cmd.Flags().GetString("sort")
	asJSON, _ := cmd.Flags().GetBool("json")

	// --- Bundled clone templates (always shown, no network required) ---
	if !asJSON {
		fmt.Printf("\n  %s Bundled templates (embedded — no network required)\n\n",
			ui.C(ui.Blue, ui.IconInfo))
		for _, name := range clone.All() {
			m, err := clone.GetManifest(name)
			if err != nil {
				fmt.Printf("  %-22s v?.?.?\n", name)
				continue
			}
			plugins := strings.Join(m.RequiredPlugins, ", ")
			fmt.Printf("  %-22s v%-8s  plugins: %s\n", m.Name, m.Version, plugins)
			fmt.Printf("  %s\n\n", ui.C(ui.Dim, m.Description))
		}
		fmt.Printf("  Scaffold: %s\n\n", ui.C(ui.Cyan, "nself init --template <name> [directory]"))
	}

	// --- Registry templates (network, may fail gracefully) ---
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	params := url.Values{}
	if category != "" {
		params.Set("category", category)
	}
	if freeOnly {
		params.Set("price_max", "0")
	}
	if sortBy != "" {
		params.Set("sort", sortBy)
	}

	baseURL := resolveTemplateRegistryURL()
	registryTemplates, err := fetchTemplateList(ctx, baseURL, params)
	if err != nil {
		// Registry unavailable is non-fatal; bundled list above already printed.
		if jsonEnvelopeOn(cmd) {
			fmt.Fprintf(os.Stderr, "community registry unavailable: %v\n", err)
			return emitEnv(cmd, newTemplateListData(nil), false)
		}
		if !asJSON {
			fmt.Fprintf(os.Stderr, "  %s Community registry unavailable: %v\n\n",
				ui.C(ui.Yellow, ui.IconWarning), err)
		}
		return nil
	}

	if jsonEnvelopeOn(cmd) {
		return emitEnv(cmd, newTemplateListData(registryTemplates), false)
	}
	if len(registryTemplates) == 0 {
		if !asJSON {
			fmt.Fprintln(os.Stderr, "No community templates found matching the given filters.")
		}
		return nil
	}

	if asJSON {
		var combined []any
		for _, n := range clone.All() {
			m, _ := clone.GetManifest(n)
			combined = append(combined, map[string]any{
				"source":  "bundled",
				"name":    m.Name,
				"version": m.Version,
				"plugins": m.RequiredPlugins,
			})
		}
		for _, t := range registryTemplates {
			combined = append(combined, t)
		}
		return ui.PrintJSON(combined)
	}

	fmt.Printf("  %s Community templates (nself.org/templates)\n\n", ui.C(ui.Blue, ui.IconInfo))
	renderTemplateTable(registryTemplates)
	fmt.Fprintf(os.Stderr, "\n%d community template(s). Browse: https://nself.org/templates\n", len(registryTemplates))
	return nil
}

func runTemplateInfo(cmd *cobra.Command, args []string) error {
	slug := args[0]
	asJSON, _ := cmd.Flags().GetBool("json")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	baseURL := resolveTemplateRegistryURL()
	t, err := fetchTemplateSingle(ctx, baseURL, slug)
	if err != nil {
		return err
	}

	if jsonEnvelopeOn(cmd) {
		return emitEnv(cmd, t, true)
	}
	if asJSON {
		return ui.PrintJSON(t)
	}

	tbl := ui.NewTable("Field", "Value")
	tbl.AddRow("Slug", t.Slug)
	tbl.AddRow("Name", t.DisplayName)
	tbl.AddRow("Description", t.Description)
	tbl.AddRow("Category", t.Category)
	tbl.AddRow("Author", t.Author)
	tbl.AddRow("Version", t.TemplateVersion)
	tbl.AddRow("CLI min", t.CliMinVersion)
	price := "Free"
	if t.PriceUSD > 0 {
		price = fmt.Sprintf("$%.2f one-time", t.PriceUSD)
	}
	tbl.AddRow("Price", price)
	if t.RatingAvg > 0 {
		tbl.AddRow("Rating", fmt.Sprintf("%.1f (%d reviews)", t.RatingAvg, t.RatingCount))
	}
	tbl.AddRow("Installs", fmt.Sprintf("%d", t.InstallCount))
	if len(t.RequiredPlugins) > 0 {
		tbl.AddRow("Required plugins", strings.Join(t.RequiredPlugins, ", "))
	}
	if t.PreviewURL != "" {
		tbl.AddRow("Preview", t.PreviewURL)
	}
	tbl.Render()

	fmt.Printf("\nInstall: nself init --template %s [dest-dir]\n", t.Slug)
	fmt.Printf("Web:     https://nself.org/templates/%s\n", t.Slug)
	return nil
}

// validateTemplateManifest performs minimal YAML/JSON manifest validation.
// It checks for required fields without importing a YAML library so we stay
// dependency-free (the manifest is small enough to parse with string checks).
func validateTemplateManifest(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading manifest %q: %w", path, err)
	}

	content := string(data)
	required := []string{"slug:", "display_name:", "version:", "category:"}
	for _, field := range required {
		if !strings.Contains(content, field) {
			return fmt.Errorf("manifest missing required field %q", strings.TrimSuffix(field, ":"))
		}
	}
	return nil
}
