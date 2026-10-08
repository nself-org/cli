package commands

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/plugin/scaffold"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// uiInfof formats and prints an info message.
func uiInfof(format string, args ...any) { ui.Info(fmt.Sprintf(format, args...)) }

// uiSuccessf formats and prints a success message.
func uiSuccessf(format string, args ...any) { ui.Success(fmt.Sprintf(format, args...)) }

// uiDimmedf formats and prints a dimmed message.
func uiDimmedf(format string, args ...any) { ui.Dimmed(fmt.Sprintf(format, args...)) }

// uiWarnf formats and prints a warning message.
func uiWarnf(format string, args ...any) { ui.Warn(fmt.Sprintf(format, args...)) }

// pluginInitCmd is the canonical scaffold command.
var pluginInitCmd = &cobra.Command{
	Use:   "init <name>",
	Short: "Scaffold a new plugin project",
	Long: `Create a new plugin project with all required files.

This command uses the canonical shared scaffold package so that output is
identical to the standalone new-plugin binary shipped with plugin-sdk-go.

Templates: go (default), rust, node, static

  nself plugin init my-plugin                          # Go plugin (free tier)
  nself plugin init my-plugin --tier licensed          # Licensed Go plugin
  nself plugin init my-plugin --template rust          # Rust plugin
  nself plugin init my-plugin --template node          # Node.js plugin
  nself plugin init my-plugin --bundle nClaw --tier licensed

Multi-tenancy (--tenancy):
  app-isolation  Emit source_account_id column (per-app isolation within one deploy)
  cloud-tenant   Emit tenant_id column + Hasura row filter (Cloud SaaS tenancy)
  both           Emit both columns; remove the unused one before production
  none           No tenancy columns (default)

When --tenancy is omitted, an interactive prompt guides the choice.
Pass --no-interactive to skip the prompt and default to none.`,
	Args:    cobra.ExactArgs(1),
	Aliases: []string{"scaffold"},
	RunE:    runPluginInit,
}

// pluginNewCmd is a deprecated alias for pluginInitCmd.
var pluginNewCmd = &cobra.Command{
	Use:        "new <name>",
	Short:      "Scaffold a new plugin project (deprecated: use 'init')",
	Long:       `Deprecated alias for 'nself plugin init'. Use 'nself plugin init' instead.`,
	Args:       cobra.ExactArgs(1),
	Deprecated: "use 'nself plugin init' instead",
	RunE:       runPluginInit,
	Hidden:     false,
}

func init() {
	addScaffoldFlags(pluginInitCmd)
	addScaffoldFlags(pluginNewCmd)
	pluginCmd.AddCommand(pluginInitCmd)
	pluginCmd.AddCommand(pluginNewCmd)
}

func addScaffoldFlags(cmd *cobra.Command) {
	cmd.Flags().String("template", "go", "Plugin template: go, rust, node, static")
	cmd.Flags().String("tier", "free", "Plugin licence: free or licensed")
	cmd.Flags().String("bundle", "", "Bundle display name (e.g. nClaw) — Licensed plugins only")
	cmd.Flags().String("description", "", "Short description of the plugin")
	cmd.Flags().String("author", "", "Plugin author name")
	cmd.Flags().String("category", "custom", "Plugin category (see F04-PLUGIN-INVENTORY-PRO)")
	cmd.Flags().String("min-cli", "1.0.9", "Minimum nSelf CLI version required")
	cmd.Flags().String("min-sdk", "0.1.0", "Minimum plugin-sdk-go version required")
	cmd.Flags().Int("port", 8080, "Default listen port")
	cmd.Flags().String("out", "", "Output directory (default ./<name>)")
	cmd.Flags().Bool("force", false, "Overwrite files if the output directory already exists")
	cmd.Flags().String("tenancy", "", "Multi-tenancy mode: app-isolation, cloud-tenant, both, none")
	cmd.Flags().Bool("no-interactive", false, "Skip interactive prompts; use --tenancy default (none)")
}

func runPluginInit(cmd *cobra.Command, args []string) error {
	name := args[0]
	tmplType, _ := cmd.Flags().GetString("template")
	tier, _ := cmd.Flags().GetString("tier")
	if tier == plugin.LicenseLicensed {
		tier = plugin.WireTierPro
	}
	bundle, _ := cmd.Flags().GetString("bundle")
	description, _ := cmd.Flags().GetString("description")
	author, _ := cmd.Flags().GetString("author")
	category, _ := cmd.Flags().GetString("category")
	minCLI, _ := cmd.Flags().GetString("min-cli")
	minSDK, _ := cmd.Flags().GetString("min-sdk")
	port, _ := cmd.Flags().GetInt("port")
	outDir, _ := cmd.Flags().GetString("out")
	force, _ := cmd.Flags().GetBool("force")
	tenancyFlag, _ := cmd.Flags().GetString("tenancy")
	noInteractive, _ := cmd.Flags().GetBool("no-interactive")

	// Validate template.
	switch tmplType {
	case "go", "rust", "node", "static":
	default:
		return fmt.Errorf("unknown template %q: must be go, rust, node, or static", tmplType)
	}

	// Resolve tenancy mode: flag wins; otherwise prompt unless --no-interactive.
	tenancy, err := resolveTenancy(tenancyFlag, noInteractive)
	if err != nil {
		return err
	}

	uiInfof("Scaffolding %s plugin %q (license: %s, tenancy: %s)...", tmplType, name, plugin.Label(tier), tenancy)

	result, err := scaffold.Run(scaffold.Options{
		Name:        name,
		Tier:        tier,
		Bundle:      bundle,
		Description: description,
		Author:      author,
		Category:    category,
		Language:    tmplType,
		MinCLI:      minCLI,
		MinSDK:      minSDK,
		Port:        port,
		OutDir:      outDir,
		Force:       force,
		Tenancy:     tenancy,
	})
	if err != nil {
		return fmt.Errorf("scaffold failed: %w", err)
	}

	uiSuccessf("Plugin %q scaffolded in %s/", name, result.Dir)
	uiDimmedf("Files created: %s", strings.Join(result.Files, ", "))
	ui.Dimmed("")
	ui.Dimmed("Next steps:")
	uiDimmedf("  cd %s", result.Dir)
	switch tmplType {
	case "go":
		ui.Dimmed("  go mod tidy && go test ./...")
		uiDimmedf("  nself plugin link %s    # link for development", result.Dir)
		uiDimmedf("  nself plugin dev %s     # start hot-reload watcher", name)
	case "rust":
		ui.Dimmed("  cargo build")
	case "node":
		ui.Dimmed("  pnpm install && pnpm build")
	case "static":
		ui.Dimmed("  # Edit static/ directory with your content")
	}

	return nil
}

// resolveTenancy returns the TenancyMode to use.
// Priority: explicit --tenancy flag > interactive prompt > default (none).
func resolveTenancy(flagVal string, noInteractive bool) (scaffold.TenancyMode, error) {
	// Explicit flag wins.
	if flagVal != "" {
		switch scaffold.TenancyMode(flagVal) {
		case scaffold.TenancyNone, scaffold.TenancyAppIsolation, scaffold.TenancyCloudTenant, scaffold.TenancyBoth:
			return scaffold.TenancyMode(flagVal), nil
		default:
			return "", fmt.Errorf("--tenancy must be one of: none, app-isolation, cloud-tenant, both; got %q", flagVal)
		}
	}

	// Non-interactive: skip prompt.
	if noInteractive || !isTerminal() {
		return scaffold.TenancyNone, nil
	}

	// Interactive prompt.
	return promptTenancy()
}

// promptTenancy runs the interactive multi-tenancy selection prompt on stdin.
func promptTenancy() (scaffold.TenancyMode, error) {
	r := bufio.NewReader(os.Stdin)

	fmt.Fprint(os.Stderr, "? Does this plugin store per-user or per-app data in Postgres tables? (y/N) ")
	ans, err := r.ReadString('\n')
	if err != nil {
		return scaffold.TenancyNone, fmt.Errorf("reading input: %w", err)
	}
	ans = strings.TrimSpace(strings.ToLower(ans))
	if ans != "y" && ans != "yes" {
		return scaffold.TenancyNone, nil
	}

	fmt.Fprintln(os.Stderr, "? What kind of isolation do you need?")
	fmt.Fprintln(os.Stderr, "  [a] Per-app isolation within one nSelf deploy  (source_account_id TEXT NOT NULL DEFAULT 'primary')")
	fmt.Fprintln(os.Stderr, "  [b] Cloud multi-tenant (separate paying customers)  (tenant_id UUID + Hasura row filter)")
	fmt.Fprintln(os.Stderr, "  [c] Not sure — add both, I'll decide later")
	fmt.Fprintln(os.Stderr, "  [d] Neither — skip tenancy scaffold")
	fmt.Fprint(os.Stderr, "Choice [a/b/c/d]: ")

	choice, err := r.ReadString('\n')
	if err != nil {
		return scaffold.TenancyNone, fmt.Errorf("reading input: %w", err)
	}
	choice = strings.TrimSpace(strings.ToLower(choice))
	switch choice {
	case "a":
		return scaffold.TenancyAppIsolation, nil
	case "b":
		return scaffold.TenancyCloudTenant, nil
	case "c":
		return scaffold.TenancyBoth, nil
	default:
		return scaffold.TenancyNone, nil
	}
}
