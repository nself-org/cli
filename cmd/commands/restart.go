package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/health"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var restartCmd = &cobra.Command{
	Use:   "restart [SERVICES...]",
	Short: "Smart restart with config change detection",
	Long: `Restart the nSelf stack with intelligent change detection.

Smart mode (default) compares .env modification time against container start
times and only rebuilds when configuration has changed. Use --all for a full
stop + start cycle.

Examples:
  nself restart              # Smart restart (rebuild only if .env changed)
  nself restart postgres     # Restart specific service
  nself restart --all        # Full stop + start cycle
  nself restart --no-build   # Skip image rebuild`,
	RunE: runRestart,
}

// restartSettleWait is the pause before the health check after a restart, and
// restartDownWait the pause between stopping and starting the stack in a full
// restart. Tests shorten both (D-0736: the lifecycle test waited 5 s per case).
var (
	restartSettleWait = 5 * time.Second
	restartDownWait   = 2 * time.Second
)

func init() {
	restartCmd.Flags().BoolP("smart", "s", true, "Detect changes, only restart affected (default)")
	restartCmd.Flags().BoolP("all", "a", false, "Full restart: stop all + start all")
	restartCmd.Flags().BoolP("verbose", "v", false, "Detailed output")
	restartCmd.Flags().Bool("skip-build", false, "Skip image rebuild")
	restartCmd.Flags().Bool("no-build", false, "Alias for --skip-build")

	RootCmd.AddCommand(restartCmd)
}

// runRestart restarts the stack; a daemon-unreachable failure is E002 in v1.5 JSON.
func runRestart(cmd *cobra.Command, args []string) error {
	return lifecycleFailure(cmd, "restart", runRestartFlow(cmd, args))
}

func runRestartFlow(cmd *cobra.Command, args []string) error {
	ui.CommandHeader("nself restart", "Smart restart with change detection")

	smart, _ := cmd.Flags().GetBool("smart")
	all, _ := cmd.Flags().GetBool("all")
	verbose, _ := cmd.Flags().GetBool("verbose")
	skipBuild, _ := cmd.Flags().GetBool("skip-build")
	noBuild, _ := cmd.Flags().GetBool("no-build")

	// --no-build is an alias for --skip-build
	if noBuild {
		skipBuild = true
	}

	// --all overrides --smart
	if all {
		smart = false
	}

	workdir, err := os.Getwd()
	if err != nil {
		ui.Error("Failed to determine working directory")
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Step 1: Load environment and validate
	totalSteps := 4
	if all {
		totalSteps = 5
	}
	stepNum := 1

	ui.Step(stepNum, totalSteps, "Loading environment")
	stepNum++

	composePath := filepath.Join(workdir, "docker-compose.yml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		ui.Error("docker-compose.yml not found. Run 'nself build' first.")
		return fmt.Errorf("docker-compose.yml not found in %s", workdir)
	}

	cfg, err := config.Load(workdir)
	if err != nil {
		ui.Error(fmt.Sprintf("Failed to load config: %v", err))
		return fmt.Errorf("loading config: %w", err)
	}

	if verbose {
		ui.Info(fmt.Sprintf("Project: %s", cfg.ProjectName))
		ui.Info(fmt.Sprintf("Working directory: %s", workdir))
		ui.Info(fmt.Sprintf("Mode: smart=%t all=%t skip-build=%t", smart, all, skipBuild))
	}

	// Read compose file manifest for multi-file support.
	composeFiles, err := build.ReadComposeManifest(workdir)
	if err != nil {
		if verbose {
			ui.Warn(fmt.Sprintf("Could not read compose manifest: %v (using defaults)", err))
		}
		composeFiles = nil
	}

	comp := docker.NewCompose(composeFiles...)
	comp.EnvFiles = build.ComposeEnvFiles(workdir)
	ctx := cmd.Context()
	services := args

	// Step 2: Determine restart strategy
	ui.Step(stepNum, totalSteps, "Determining restart strategy")
	stepNum++

	mode := restartMode(all, smart, services)
	var before restartBefore
	if lifecycleJSON(cmd) {
		before = snapshotRestart(ctx, comp, workdir, cfg.ProjectName)
	}
	if all {
		// Full mode: stop all, wait, start all
		if err := restartFull(ctx, comp, workdir, cfg, verbose, skipBuild, stepNum, totalSteps); err != nil {
			return err
		}
	} else if len(services) > 0 {
		// Specific services: restart only those
		if err := restartServices(ctx, comp, workdir, cfg, services, verbose); err != nil {
			return err
		}
	} else if smart {
		// Smart mode: check if config changed
		if err := restartSmart(ctx, comp, workdir, cfg, verbose, skipBuild); err != nil {
			return err
		}
	} else {
		// Default: quick compose restart
		if err := restartQuick(ctx, comp, workdir, verbose); err != nil {
			return err
		}
	}

	// Health verification
	ui.Step(totalSteps, totalSteps, "Verifying health")

	if verbose {
		ui.Info("Waiting 5 seconds for services to stabilize...")
	}
	time.Sleep(restartSettleWait)

	report, err := health.RunAllChecks(ctx, cfg, workdir)
	if err != nil {
		ui.Warn(fmt.Sprintf("Health check error: %v", err))
	} else {
		// Same predicate as report.Unhealthy below (health.HealthResult.OK) —
		// see issue #268: a per-service loop that only accepted "healthy"
		// disagreed with an aggregate that also accepted "running".
		for _, r := range report.Results {
			if r.OK() {
				ui.Success(fmt.Sprintf("%-20s %s (%s)", r.Service, r.Status, r.Duration.Round(time.Millisecond)))
			} else {
				ui.Warn(fmt.Sprintf("%-20s %s (%s)", r.Service, r.Status, r.Details))
			}
		}
		fmt.Println()
		if report.Unhealthy > 0 {
			ui.Warn(fmt.Sprintf("%d/%d services healthy, %d unhealthy", report.Healthy, report.Total, report.Unhealthy))
		} else {
			ui.Success(fmt.Sprintf("All %d services healthy", report.Total))
		}
	}

	if !lifecycleJSON(cmd) {
		return nil
	}
	return emitLifecycle(cmd, "restart", RestartResult{Restarted: restartedServices(ctx, comp, workdir, mode, services, before), Mode: mode})
}

// runningServices names the compose services of the containers `stop` found
// running before it ran compose down; never nil.
func runningServices(containers []docker.ContainerInfo) []string {
	names := []string{}
	for _, c := range containers {
		name := c.Service
		if name == "" {
			name = c.Name
		}
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// restartMode names the strategy runRestart dispatches to, in its own order:
// --all, then named services, then smart (the default), else quick.
func restartMode(all, smart bool, services []string) string {
	switch {
	case all:
		return "all"
	case len(services) > 0:
		return "services"
	case smart:
		return "smart"
	}
	return "quick"
}

// restartBefore is what a restart can see before its strategy runs: the
// configured services that have a container in any state (compose restart
// touches stopped ones too) and the compose file stamp, which a rebuild changes.
type restartBefore struct {
	existing []string
	compose  string
}

func composeStamp(workdir string) string {
	info, err := os.Stat(filepath.Join(workdir, "docker-compose.yml"))
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d/%d", info.Size(), info.ModTime().UnixNano())
}

// snapshotRestart lists the services of the compose manifest that have a
// container of this project, in manifest order; orphans and configured services
// without a container are left out.
func snapshotRestart(ctx context.Context, comp *docker.Compose, workdir, project string) restartBefore {
	before := restartBefore{existing: []string{}, compose: composeStamp(workdir)}
	containers, err := docker.RunningConfigHashes(ctx, project)
	if err != nil {
		return before
	}
	have := map[string]bool{}
	for _, c := range containers {
		have[c.Service] = true
	}
	var configured []string
	_ = config.WithComposeEnvironment(func() error {
		var queryErr error
		configured, queryErr = comp.ComposeServiceNames(ctx, workdir)
		return queryErr
	})
	for _, name := range configured {
		if have[name] {
			before.existing = append(before.existing, name)
		}
	}
	return before
}

// restartedServices names what a finished restart restarted, never nil, by the
// branch runRestart took (all wins over named services): the whole compose
// manifest after --all (down, then up of everything) or after a smart rebuild
// (the compose file changed, up applied it); the services asked for; else the
// services that had a container, the only ones compose restart touches.
func restartedServices(ctx context.Context, comp *docker.Compose, workdir, mode string, services []string, before restartBefore) []string {
	if mode == "all" || (mode != "services" && composeStamp(workdir) != before.compose) {
		var names []string
		err := config.WithComposeEnvironment(func() error {
			var queryErr error
			names, queryErr = comp.ComposeServiceNames(ctx, workdir)
			return queryErr
		})
		if err == nil && len(names) > 0 {
			return names
		}
	}
	if mode == "services" {
		return services
	}
	return before.existing
}
