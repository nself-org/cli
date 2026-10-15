package commands

// Purpose: `nself logs` reporting subcommands split out of logs.go
// (CLI-R12 Batch B mechanical file-size split). Holds the --status,
// --summary, and --top flag handlers, dispatched from runLogs in logs.go.
// Inputs: the cobra command/context and (for --summary) an optional
// positional service name.
// Outputs: rendered tables/output on stdout; errors wrap docker compose
// failures.
// Constraints: pure move, no behavior change.

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// runLogsStatus shows a service status overview using docker compose ps.
func runLogsStatus(cmd *cobra.Command) error {
	rawCwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	workdir, err := config.FindNSelfRoot(rawCwd)
	if err != nil {
		return fmt.Errorf("no nself project found in current directory or parents. Run 'nself init' to create a project")
	}

	ui.CommandHeader("nself logs --status", "Service status overview")

	dockerCmd := docker.NewProjectCompose(workdir).CommandContext(cmd.Context(), workdir, "ps")
	dockerCmd.Dir = workdir
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr

	if err := dockerCmd.Run(); err != nil {
		return fmt.Errorf("docker compose ps: %w", err)
	}

	return nil
}

// runLogsSummary shows recent errors across services.
func runLogsSummary(cmd *cobra.Command, args []string) error {
	rawCwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	workdir, err := config.FindNSelfRoot(rawCwd)
	if err != nil {
		return fmt.Errorf("no nself project found in current directory or parents. Run 'nself init' to create a project")
	}

	ui.CommandHeader("nself logs --summary", "Recent errors by service")

	var services []string
	if len(args) > 0 {
		services = []string{args[0]}
	} else {
		var err error
		services, err = getRunningServices(cmd.Context(), workdir)
		if err != nil {
			return fmt.Errorf("listing services: %w", err)
		}
	}

	tbl := ui.NewTable("Service", "Lines", "Errors", "Warnings", "Last Error")
	for _, svc := range services {
		summary, err := CollectLogSummary(cmd.Context(), workdir, svc, 500)
		if err != nil {
			return fmt.Errorf("collecting log summary for %s: %w", svc, err)
		}

		lastError := "—"
		if len(summary.TopErrors) > 0 {
			msg := summary.TopErrors[0]
			if len(msg) > 50 {
				msg = msg[:50]
			}
			lastError = msg
		}

		tbl.AddRow(
			summary.Service,
			strconv.Itoa(summary.TotalLines),
			strconv.Itoa(summary.ErrorCount),
			strconv.Itoa(summary.WarnCount),
			lastError,
		)
	}
	tbl.Render()

	return nil
}

// runLogsTop shows the most active services by log volume.
func runLogsTop(cmd *cobra.Command) error {
	rawCwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	workdir, err := config.FindNSelfRoot(rawCwd)
	if err != nil {
		return fmt.Errorf("no nself project found in current directory or parents. Run 'nself init' to create a project")
	}

	ui.CommandHeader("nself logs --top", "Most active services")

	services, err := getRunningServices(cmd.Context(), workdir)
	if err != nil {
		return fmt.Errorf("listing services: %w", err)
	}

	volumes := make([]*LogVolume, 0, len(services))
	for _, svc := range services {
		vol, err := SampleLogVolume(cmd.Context(), workdir, svc, 30*time.Second)
		if err != nil {
			return fmt.Errorf("sampling log volume for %s: %w", svc, err)
		}
		volumes = append(volumes, vol)
	}

	sort.Slice(volumes, func(i, j int) bool {
		return volumes[i].LinesPerMin > volumes[j].LinesPerMin
	})

	tbl := ui.NewTable("Service", "Lines/min", "Bytes/min", "Error Rate")
	for _, vol := range volumes {
		tbl.AddRow(
			vol.Service,
			fmt.Sprintf("%.1f", vol.LinesPerMin),
			fmt.Sprintf("%.1f", vol.BytesPerMin),
			fmt.Sprintf("%.1f%%", vol.ErrorRate*100),
		)
	}
	tbl.Render()

	return nil
}
