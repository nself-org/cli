package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/nself-org/cli/internal/health"

	"github.com/spf13/cobra"
)

// Flags shared across health subcommands.
var (
	healthTimeout  int
	healthInterval int
	healthRetries  int
	healthEnv      string
	healthJSON     bool
	healthQuiet    bool
)

var healthCmd = &cobra.Command{
	Use:   "health [subcommand]",
	Short: "Health check management with continuous monitoring",
	Long: `Run health checks against all services, individual services, or HTTP
endpoints. Supports continuous watch mode and history viewing.`,
	// Default action when no subcommand is given: run all checks.
	RunE: healthCheckRunE,
}

var healthCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Run all health checks",
	RunE:  healthCheckRunE,
}

var healthServiceCmd = &cobra.Command{
	Use:   "service <name>",
	Short: "Check a single service",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(healthTimeout)*time.Second)
		defer cancel()

		var lastErr error
		var result *health.HealthResult
		for attempt := 0; attempt < healthRetries; attempt++ {
			result, lastErr = health.CheckService(ctx, args[0])
			if lastErr == nil && result.OK() {
				break
			}
		}
		if lastErr != nil {
			return fmt.Errorf("checking service %s: %w", args[0], lastErr)
		}

		if healthJSON {
			return emitHealthJSON("status health service", result)
		}
		if healthQuiet && result.OK() {
			return nil
		}
		printServiceResult(result)
		return nil
	},
}

var healthEndpointCmd = &cobra.Command{
	Use:   "endpoint <url>",
	Short: "Check an HTTP endpoint",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(healthTimeout)*time.Second)
		defer cancel()

		var lastErr error
		var result *health.HealthResult
		for attempt := 0; attempt < healthRetries; attempt++ {
			result, lastErr = health.CheckEndpoint(ctx, args[0])
			if lastErr == nil && result.OK() {
				break
			}
		}
		if lastErr != nil {
			return fmt.Errorf("checking endpoint %s: %w", args[0], lastErr)
		}

		if healthJSON {
			return emitHealthJSON("status health endpoint", result)
		}
		if healthQuiet && result.OK() {
			return nil
		}
		printServiceResult(result)
		return nil
	},
}

var healthWatchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Continuous health monitoring",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, workdir, err := loadHealthConfig()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		interval := time.Duration(healthInterval) * time.Second
		ch, err := health.WatchHealth(ctx, cfg, workdir, interval)
		if err != nil {
			return fmt.Errorf("starting watch: %w", err)
		}

		for report := range ch {
			if healthQuiet && report.Unhealthy == 0 {
				continue
			}
			if healthJSON {
				if err := printJSON(report); err != nil {
					return err
				}
			} else {
				printReport(report)
			}
		}
		return nil
	},
}

var healthHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "Show last 20 health checks",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := healthHistoryDir()
		if err != nil {
			return err
		}

		entries, err := health.GetHistory(dir, 20)
		if err != nil {
			return fmt.Errorf("reading history: %w", err)
		}

		if healthJSON {
			return emitHealthHistory(entries)
		}
		if len(entries) == 0 {
			fmt.Println("No health check history found.")
			return nil
		}

		for _, report := range entries {
			fmt.Printf("[%s] %d/%d healthy\n",
				report.Timestamp.Format(time.RFC3339),
				report.Healthy,
				report.Total,
			)
			for _, r := range report.Results {
				printServiceResult(&r)
			}
			fmt.Println()
		}
		return nil
	},
}

var healthConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Show health check settings",
	RunE: func(cmd *cobra.Command, args []string) error {
		settings := healthConfigData{
			Env:             healthEnv,
			IntervalSeconds: healthInterval,
			JSONOutput:      healthJSON,
			Quiet:           healthQuiet,
			Retries:         healthRetries,
			TimeoutSeconds:  healthTimeout,
		}
		if healthJSON {
			return emitHealthJSON("status health config", settings)
		}
		fmt.Printf("%-20s %v\n", "Timeout (seconds):", healthTimeout)
		fmt.Printf("%-20s %v\n", "Interval (seconds):", healthInterval)
		fmt.Printf("%-20s %v\n", "Retries:", healthRetries)
		fmt.Printf("%-20s %s\n", "Environment:", healthEnv)
		fmt.Printf("%-20s %v\n", "JSON output:", healthJSON)
		fmt.Printf("%-20s %v\n", "Quiet:", healthQuiet)
		return nil
	},
}
