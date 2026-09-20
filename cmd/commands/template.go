package commands

import (
	"github.com/spf13/cobra"
)

// defaultTemplateRegistryURL is the base URL for the template registry API.
const defaultTemplateRegistryURL = "https://nself.org/api/templates"

// templateEntry is a single template from the registry.
type templateEntry struct {
	Slug            string   `json:"slug"`
	DisplayName     string   `json:"displayName"`
	Description     string   `json:"description"`
	Category        string   `json:"category"`
	Author          string   `json:"author"`
	PriceUSD        float64  `json:"priceUsd"`
	RatingAvg       float64  `json:"ratingAvg"`
	RatingCount     int      `json:"ratingCount"`
	InstallCount    int      `json:"installCount"`
	RequiredPlugins []string `json:"requiredPlugins"`
	PreviewURL      string   `json:"previewUrl"`
	TemplateVersion string   `json:"templateVersion"`
	CliMinVersion   string   `json:"cliMinVersion"`
	TarballURL      string   `json:"tarballUrl"`
	TarballSHA256   string   `json:"tarballSha256"`
}

// templateListResponse is the top-level shape of GET /api/templates.
type templateListResponse struct {
	Templates []templateEntry `json:"templates"`
	Total     int             `json:"total"`
}

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Browse and publish full-stack app templates",
	Long: `Browse, install, and publish community full-stack app templates.

Templates include Postgres schema, Hasura metadata, seed data, and a Flutter starter.
Browse at nself.org/templates or install from the CLI:

  nself template list
  nself template info saas-starter
  nself init --template saas-starter ./my-app
  nself template publish`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself template` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// --- list subcommand ---

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available app templates",
	Long: `Fetch and display all app templates available in the nSelf registry.

  nself template list
  nself template list --category saas
  nself template list --free
  nself template list --json`,
	RunE: runTemplateList,
}

// --- info subcommand ---

var templateInfoCmd = &cobra.Command{
	Use:   "info <slug>",
	Short: "Show detail for a single template",
	Long: `Display full detail for a template including required plugins and install command.

  nself template info saas-starter
  nself template info saas-starter --json`,
	Args: cobra.ExactArgs(1),
	RunE: runTemplateInfo,
}

// --- publish subcommand ---

var templatePublishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Submit a template to the nSelf registry",
	Long: `Validate your template manifest and upload it to the nSelf template registry.

The current directory must contain a valid template.yml manifest and a built tarball:

  nself template publish
  nself template publish --tarball ./dist/my-template.tar.gz --manifest template.yml`,
	RunE: runTemplatePublish,
}

// --- update subcommand ---

var templateUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Apply incremental migrations for the installed template",
	Long: `Run incremental schema migrations from the template's migrations/ directory.

Only additive changes are applied automatically. Destructive changes require --force
and explicit confirmation before running.

  nself template update
  nself template update --force`,
	RunE: runTemplateUpdate,
}

func init() {
	// list flags
	templateListCmd.Flags().String("category", "", "Filter by category: saas, marketplace, social, productivity, media, ecommerce")
	templateListCmd.Flags().Bool("free", false, "Show free templates only")
	templateListCmd.Flags().String("sort", "installs", "Sort by: installs, rating, newest, price")
	templateListCmd.Flags().Bool("json", false, "Output raw JSON")

	// info flags
	templateInfoCmd.Flags().Bool("json", false, "Output raw JSON")

	// publish flags
	templatePublishCmd.Flags().String("tarball", "", "Path to the compiled template tarball (.tar.gz)")
	templatePublishCmd.Flags().String("manifest", "template.yml", "Path to the template manifest file")

	// update flags
	templateUpdateCmd.Flags().Bool("force", false, "Allow destructive migrations (requires confirmation)")
	templateUpdateCmd.Flags().Bool("dry-run", false, "Show pending migrations without applying them")

	// register subcommands
	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateInfoCmd)
	templateCmd.AddCommand(templatePublishCmd)
	templateCmd.AddCommand(templateUpdateCmd)

	RootCmd.AddCommand(templateCmd)
}
