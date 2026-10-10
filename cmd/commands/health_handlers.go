package commands

// Purpose: RunE handlers, output-formatting helpers, and the cobra AddCommand wiring for the nself health command group defined in health.go.
// Inputs: cobra command flags shared across health subcommands.
// Outputs: formatted health check/report output, and the registered command tree.
// Constraints: split out of health.go as a pure move (CLI-R12); no behavior change.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/health"
	"github.com/nself-org/cli/internal/output"

	"github.com/spf13/cobra"
)

// healthCheckRunE is the default action: run all service health checks.
func healthCheckRunE(cmd *cobra.Command, args []string) error {
	cfg, workdir, err := loadHealthConfig()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(healthTimeout)*time.Second)
	defer cancel()

	var report *health.HealthReport
	var lastErr error
	for attempt := 0; attempt < healthRetries; attempt++ {
		report, lastErr = health.RunAllChecks(ctx, cfg, workdir)
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		return fmt.Errorf("running health checks: %w", lastErr)
	}

	if healthJSON {
		return emitHealthJSON(healthCommandName(cmd), report)
	}
	if healthQuiet && report.Unhealthy == 0 {
		return nil
	}
	printReport(report)
	return nil
}

// loadHealthConfig loads the project config, optionally setting ENV from --env.
// Returns the config and the project working directory.
func loadHealthConfig() (*config.Config, string, error) {
	if healthEnv != "" {
		_ = os.Setenv("ENV", healthEnv)
	}
	dir, err := os.Getwd()
	if err != nil {
		return nil, "", fmt.Errorf("getting working directory: %w", err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return nil, "", fmt.Errorf("loading config: %w", err)
	}
	return cfg, dir, nil
}

// healthHistoryDir returns the directory where health history is stored.
func healthHistoryDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	return filepath.Join(dir, ".nself", "health"), nil
}

// printReport prints a HealthReport as a formatted table.
func printReport(report *health.HealthReport) {
	fmt.Printf("%-20s %-12s %-8s %s\n", "Service", "Status", "Time", "Details")
	for _, r := range report.Results {
		printServiceResult(&r)
	}
	fmt.Printf("\n%d/%d services healthy\n", report.Healthy, report.Total)
}

// printServiceResult prints a single health result line. Called both for
// RunAllChecks results (where a no-healthcheck container reports "running")
// and for CheckService/CheckEndpoint results (which never report "running"),
// so it must use the same HealthResult.OK() predicate as every other
// aggregate/per-service comparison — see issue #268.
func printServiceResult(r *health.HealthResult) {
	marker := "\u2717"
	if r.OK() {
		marker = "\u2713"
	}
	fmt.Printf("%-20s %s %-10s %-8s %s\n", r.Service, marker, r.Status, r.Duration.Truncate(time.Millisecond), r.Details)
}

// healthCommandName is the canonical (v1.5) registry path of a health handler
// that serves both `status health` and `status health check`.
func healthCommandName(cmd *cobra.Command) string {
	if cmd.Name() == "check" {
		return "status health check"
	}
	return "status health"
}

// emitHealthJSON writes a health document: the pre-contract JSON, bare in v1.4
// (byte-identical to printJSON) and the v1 envelope in v1.5 (P7-SURF-11).
func emitHealthJSON(command string, data any) error {
	return output.EmitLegacyCompatible(pilotWriter(), command, data)
}

// emitHealthHistory writes `status health history --json`. The bare shape is
// the array of reports; the envelope data wraps it as `entries`, never null.
// With no history the pre-contract output was a text line, not JSON; that
// stays so in v1.4 mode and with NSELF_JSON_LEGACY=1, and v1.5 prints an
// envelope with an empty list.
func emitHealthHistory(entries []health.HealthReport) error {
	// compat.V15(P7-SURF-11): health history --json with no history prints a text line -> the v1 envelope with an empty entries list
	if len(entries) == 0 && (!compat.V15() || legacyJSONRequested()) {
		_, err := fmt.Fprintln(pilotWriter().Out, "No health check history found.")
		return err
	}
	if entries == nil {
		entries = []health.HealthReport{}
	}
	return emitStateJSON("status health history", entries, healthHistoryData{Entries: entries})
}

// printJSON marshals v to indented JSON and writes to stdout.
func printJSON(v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func init() {
	// Register persistent flags on the parent command so all subcommands inherit them.
	healthCmd.PersistentFlags().IntVar(&healthTimeout, "timeout", 30, "Check timeout in seconds")
	healthCmd.PersistentFlags().IntVar(&healthInterval, "interval", 10, "Watch interval in seconds")
	healthCmd.PersistentFlags().IntVar(&healthRetries, "retries", 3, "Retry count on failure")
	healthCmd.PersistentFlags().StringVar(&healthEnv, "env", "", "Environment to load config for")
	healthCmd.PersistentFlags().BoolVar(&healthJSON, "json", false, "Output in JSON format")
	healthCmd.PersistentFlags().BoolVar(&healthQuiet, "quiet", false, "Only output on failure")

	// Register subcommands.
	healthCmd.AddCommand(healthCheckCmd)
	healthCmd.AddCommand(healthServiceCmd)
	healthCmd.AddCommand(healthEndpointCmd)
	healthCmd.AddCommand(healthWatchCmd)
	healthCmd.AddCommand(healthHistoryCmd)
	healthCmd.AddCommand(healthConfigCmd)

	RootCmd.AddCommand(healthCmd)
}
