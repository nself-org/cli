package commands

import (
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var urlsCmd = &cobra.Command{
	Use:   "urls",
	Short: "Display all service URLs with route conflict detection",
	Long: `Show all dynamically assigned proxy URLs for the current project,
grouped by service type: Required, Optional, Custom, and Frontend.

URLs are computed from BASE_DOMAIN and each service's route configuration.
Use --check-conflicts to detect overlapping routes.`,
	RunE: runURLs,
}

func init() {
	urlsCmd.Flags().BoolP("all", "a", false, "Show all routes including internal")
	urlsCmd.Flags().Bool("json", false, "JSON output")
	urlsCmd.Flags().String("env", "", "Show URLs for specific environment")
	urlsCmd.Flags().String("diff", "", "Compare URLs between environments")
	urlsCmd.Flags().Bool("check-conflicts", false, "Check for route conflicts")

	RootCmd.AddCommand(urlsCmd)
}

// serviceURL represents a single resolved URL entry for display and JSON output.
type serviceURL struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Internal bool   `json:"internal,omitempty"`
	Group    string `json:"group"`
}

// urlsOutput is the JSON-serializable result of the urls command.
type urlsOutput struct {
	BaseDomain       string       `json:"base_domain"`
	Env              string       `json:"env"`
	RequiredServices []serviceURL `json:"required_services"`
	OptionalServices []serviceURL `json:"optional_services,omitempty"`
	CustomServices   []serviceURL `json:"custom_services,omitempty"`
	FrontendApps     []serviceURL `json:"frontend_apps,omitempty"`
	InternalRoutes   []serviceURL `json:"internal_routes,omitempty"`
	TotalRoutes      int          `json:"total_routes"`
	Conflicts        []conflict   `json:"conflicts,omitempty"`
}

// conflict describes two services whose routes overlap.
type conflict struct {
	Route    string `json:"route"`
	Service1 string `json:"service_1"`
	Service2 string `json:"service_2"`
}

func runURLs(cmd *cobra.Command, args []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	showAll, _ := cmd.Flags().GetBool("all")
	envFlag, _ := cmd.Flags().GetString("env")
	diffFlag, _ := cmd.Flags().GetString("diff")
	checkConflicts, _ := cmd.Flags().GetBool("check-conflicts")

	if !jsonOut {
		ui.CommandHeaderStderr("nself urls", "Service URL overview")
	}

	rawCwd, err := os.Getwd()
	if err != nil {
		if jsonOut {
			ui.PrintJSONError(err)
		} else {
			ui.Error("Failed to determine working directory")
		}
		return fmt.Errorf("getting working directory: %w", err)
	}
	workdir, err := config.FindNSelfRoot(rawCwd)
	if err != nil {
		return fmt.Errorf("no nself project found in current directory or parents. Run 'nself init' to create a project")
	}

	// If --env is set, override the ENV variable before loading config.
	if envFlag != "" {
		_ = os.Setenv("ENV", envFlag)
	}

	// --diff: load two environments and compare.
	if diffFlag != "" {
		return runURLsDiff(workdir, envFlag, diffFlag, jsonOut)
	}

	cfg, err := config.Load(workdir)
	if err != nil {
		if jsonOut {
			ui.PrintJSONError(err)
		} else {
			ui.Error(fmt.Sprintf("Failed to load config: %v", err))
		}
		return err
	}

	out := buildURLOutput(cfg, showAll)

	if checkConflicts {
		out.Conflicts = detectConflicts(out)
	}

	if jsonOut {
		// Pre-contract JSON: bare in v1.4, the v1 envelope in v1.5 (P7-SURF-11).
		return emitStateJSON("status urls", out, urlsData{urlsOutput: out})
	}

	printURLGroups(out, showAll)

	if checkConflicts && len(out.Conflicts) > 0 {
		fmt.Println()
		ui.Warn(fmt.Sprintf("%d route conflict(s) detected:", len(out.Conflicts)))
		for _, c := range out.Conflicts {
			fmt.Printf("  %s: %s vs %s\n",
				ui.C(ui.Yellow, c.Route),
				ui.C(ui.Cyan, c.Service1),
				ui.C(ui.Cyan, c.Service2),
			)
		}
	} else if checkConflicts {
		fmt.Println()
		ui.Success("No route conflicts detected")
	}

	return nil
}
