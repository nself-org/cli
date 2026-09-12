package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/health"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// statusJSONOutput is the JSON schema for --json output.
type statusJSONOutput struct {
	Timestamp string              `json:"timestamp"`
	Services  []statusJSONService `json:"services"`
	Summary   statusJSONSummary   `json:"summary"`
}

type statusJSONService struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Duration string `json:"duration"`
	Details  string `json:"details"`
}

type statusJSONSummary struct {
	Total     int `json:"total"`
	Healthy   int `json:"healthy"`
	Unhealthy int `json:"unhealthy"`
}

var statusCmd = &cobra.Command{
	Use:   "status [SERVICE]",
	Short: "Show health status of all services",
	Long: `Show health status of all nSelf services, or a single service if specified.

Exit codes:
  0  All services healthy
  1  Error running checks
  2  One or more services unhealthy`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOut, _ := cmd.Flags().GetBool("json")
		verbose, _ := cmd.Flags().GetBool("verbose")
		healthOnly, _ := cmd.Flags().GetBool("health-only")
		metrics, _ := cmd.Flags().GetBool("metrics")
		deep, _ := cmd.Flags().GetBool("deep")

		// --deep is a deep health aggregator: enables verbose + metrics and
		// includes resource usage details. For full subsystem diagnostics,
		// users should run `nself doctor --deep`.
		if deep {
			verbose = true
			metrics = true
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Single service mode.
		if len(args) == 1 {
			exitCode, err := runSingleServiceStatus(ctx, args[0], jsonOut, verbose)
			if err != nil {
				return err
			}
			if exitCode != 0 {
				cmd.Root().SetContext(context.WithValue(cmd.Root().Context(), exitCodeKey, exitCode))
			}
			return nil
		}

		// Full status: load config to discover enabled services.
		rawCwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
		cwd, err := config.FindNSelfRoot(rawCwd)
		if err != nil {
			return fmt.Errorf("no nself project found in current directory or parents. Run 'nself init' to create a project")
		}

		cfg, err := config.Load(cwd)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		report, err := health.RunAllChecks(ctx, cfg, cwd)
		if err != nil {
			return fmt.Errorf("failed to run health checks: %w", err)
		}

		if jsonOut {
			return printStatusJSON(report)
		}

		printStatusTable(report, verbose, healthOnly, metrics)

		// G-014: surface orphaned containers (running with no matching
		// service in the current compose) here too, not just at build time —
		// a container can go orphaned between builds if someone removes it
		// from nself.yaml and never reruns `nself build`. Read-only: status
		// never removes anything itself.
		printOrphanStatusHint(ctx, cwd, cfg.ProjectName, filepath.Join(cwd, "docker-compose.yml"))

		// Show installed plugin versions below the service health table.
		pluginDir := build.DefaultPluginDir()
		if plugins, pluginErr := plugin.ListInstalled(pluginDir); pluginErr == nil && len(plugins) > 0 {
			fmt.Println()
			fmt.Println("Installed plugins:")
			for _, p := range plugins {
				ver := p.Version
				if ver == "" {
					ver = "unknown"
				}
				fmt.Printf("  %-20s %s\n", p.Name, ver)
			}
		}

		// State-aware suggestions.
		printStatusSuggestions(report)

		// Exit code: 2 if any unhealthy, 1 if some in intermediate state.
		if report.Unhealthy > 0 {
			cmd.Root().SetContext(context.WithValue(cmd.Root().Context(), exitCodeKey, 2))
		} else if report.Healthy < report.Total {
			// Some services in intermediate state (starting, etc.) — warning
			cmd.Root().SetContext(context.WithValue(cmd.Root().Context(), exitCodeKey, 1))
		}
		return nil
	},
}

// runSingleServiceStatus checks a single service by name.
// Returns an exit code (0 = healthy, 2 = unhealthy) and any error.
func runSingleServiceStatus(ctx context.Context, service string, jsonOut, verbose bool) (int, error) {
	result, err := health.CheckService(ctx, service)
	if err != nil {
		return 0, wrapDockerError(err, service)
	}

	if jsonOut {
		report := &health.HealthReport{
			Timestamp: time.Now(),
			Results:   []health.HealthResult{*result},
			Total:     1,
		}
		if result.OK() {
			report.Healthy = 1
		} else {
			report.Unhealthy = 1
		}
		return 0, printStatusJSON(report)
	}

	// Simple single-service output.
	icon := ui.C(ui.Green, ui.IconSuccess)
	statusText := ui.C(ui.Green, "healthy")
	if result.Status != "healthy" {
		icon = ui.C(ui.Red, ui.IconFailure)
		statusText = ui.C(ui.Red, result.Status)
	}

	fmt.Printf("%s %s  %s", icon, statusText, result.Service)
	if verbose {
		fmt.Printf("  (%s, %s)", result.Duration.Round(time.Millisecond), result.Details)
	} else {
		fmt.Printf("  (%s)", result.Details)
	}
	fmt.Println()

	if result.Status != "healthy" {
		return 2, nil
	}
	return 0, nil
}

func init() {
	statusCmd.Flags().BoolP("json", "j", false, "JSON output")
	statusCmd.Flags().Bool("verbose", false, "Show resource usage, uptime")
	statusCmd.Flags().Bool("health-only", false, "Show only health status")
	statusCmd.Flags().Bool("metrics", false, "Show performance metrics")
	statusCmd.Flags().Bool("deep", false, "Deep health aggregator (verbose + metrics + resource usage)")
	RootCmd.AddCommand(statusCmd)
}
