package commands

// Purpose: The four restart strategies `nself restart` dispatches to based
// on what changed and the flags passed — full (stop+start cycle), targeted
// service restarts, smart (rebuild-if-stale then restart affected
// services), and quick (restart without health/rebuild checks). Split out
// of restart.go (CLI-R12) to separate the strategy implementations from
// the cobra command wiring and flag/strategy-selection logic
// (runRestart) that remain in restart.go.
// Inputs: a context.Context, a *docker.Compose, the project workdir, and
// (per strategy) a *config.Config, a service name list, verbose/skipBuild
// flags, and step-numbering for progress output.
// Outputs: the requested services are stopped/rebuilt/started; errors
// propagate to runRestart's caller.
// Constraints: pure move — no behavior changes. runRestart chooses which
// of these to call based on the --full/--quick flags and whether specific
// service names were passed.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/ui"
)

// restartFull performs a full stop + start cycle.
func restartFull(ctx context.Context, comp *docker.Compose, workdir string, cfg *config.Config, verbose, skipBuild bool, stepNum, totalSteps int) error {
	// Confirm and rebuild before stopping services: a refusal must leave the
	// running stack untouched.
	if !skipBuild {
		needsRebuild, err := build.NeedsRebuild(workdir)
		if err != nil {
			ui.Warn(fmt.Sprintf("Could not check build cache: %v", err))
		}
		if needsRebuild {
			ui.Info("Configuration changed, rebuilding...")
			if _, err := runAutomaticBuild(workdir, build.BuildOptions{}); err != nil {
				ui.Error(fmt.Sprintf("Rebuild failed: %v", err))
				return fmt.Errorf("rebuilding: %w", err)
			}
			ui.Success("Rebuild complete")
		}
	}

	ui.Step(stepNum, totalSteps, "Stopping all services")
	stepNum++
	if err := comp.ComposeDown(ctx, workdir, docker.DownOptions{RemoveOrphans: true}); err != nil {
		ui.Error(fmt.Sprintf("Failed to stop services: %v", err))
		return fmt.Errorf("stopping services: %w", err)
	}
	ui.Success("All services stopped")
	if verbose {
		ui.Info("Waiting 2 seconds before restart...")
	}
	time.Sleep(2 * time.Second)

	ui.Step(stepNum, totalSteps, "Starting all services")

	if err := comp.ComposeUp(ctx, workdir); err != nil {
		ui.Error(fmt.Sprintf("Failed to start services: %v", err))
		return fmt.Errorf("starting services: %w", err)
	}
	ui.Success("All services started")

	return nil
}

// restartServices restarts only the specified services.
func restartServices(ctx context.Context, comp *docker.Compose, workdir string, services []string, verbose bool) error {
	if verbose {
		ui.Info(fmt.Sprintf("Restarting services: %v", services))
	}
	ui.Info(fmt.Sprintf("Restarting %d service(s)...", len(services)))

	// Apply any compose change first. `docker compose restart` bounces the
	// existing container and never re-reads the compose file, so a config edit
	// followed by `nself restart <service>` silently did nothing and the user
	// had to recreate the container by hand — reported by the ntask clean-fork
	// self-host drill, 2026-08-24. `up -d --no-deps` recreates only what
	// actually changed and is a no-op otherwise.
	if err := comp.ComposeUpNoDeps(ctx, workdir, services...); err != nil {
		ui.Error(fmt.Sprintf("Failed to apply compose configuration: %v", err))
		return fmt.Errorf("applying compose configuration: %w", err)
	}

	// Then restart, so the command does what it says even when nothing in the
	// compose file changed and `up` left the containers untouched.
	if err := comp.ComposeRestart(ctx, workdir, services...); err != nil {
		ui.Error(fmt.Sprintf("Failed to restart services: %v", err))
		return fmt.Errorf("restarting services: %w", err)
	}
	ui.Success(fmt.Sprintf("Restarted: %v", services))

	return nil
}

// restartSmart compares .env mtime against container start times. If config
// is newer than any container, a full rebuild + up is performed. Otherwise,
// a quick compose restart is issued.
func restartSmart(ctx context.Context, comp *docker.Compose, workdir string, cfg *config.Config, verbose, skipBuild bool) error {
	envPath := filepath.Join(workdir, ".env")
	envInfo, err := os.Stat(envPath)
	if err != nil {
		if verbose {
			ui.Info("No .env file found, performing quick restart")
		}
		return restartQuick(ctx, comp, workdir, verbose)
	}
	envMtime := envInfo.ModTime()

	// Also check docker-compose.yml mtime
	composePath := filepath.Join(workdir, "docker-compose.yml")
	composeInfo, err := os.Stat(composePath)
	if err != nil {
		ui.Warn("docker-compose.yml not found, performing quick restart")
		return restartQuick(ctx, comp, workdir, verbose)
	}

	// Compare .env mtime and docker-compose.yml mtime: use the newer one
	configMtime := envMtime
	if composeInfo.ModTime().After(configMtime) {
		configMtime = composeInfo.ModTime()
	}

	// Get container start times
	containers, err := comp.ComposePs(ctx, workdir)
	if err != nil {
		if verbose {
			ui.Warn(fmt.Sprintf("Could not query containers: %v", err))
		}
		return restartQuick(ctx, comp, workdir, verbose)
	}

	// Check if any running container started before the config changed
	configChanged := false
	for _, c := range containers {
		info, err := docker.InspectContainer(ctx, c.Name)
		if err != nil {
			if verbose {
				ui.Warn(fmt.Sprintf("Could not inspect %s: %v", c.Name, err))
			}
			continue
		}
		startedAt, err := time.Parse(time.RFC3339Nano, info.StartedAt)
		if err != nil {
			if verbose {
				ui.Warn(fmt.Sprintf("Could not parse start time for %s: %v", c.Name, err))
			}
			continue
		}
		if configMtime.After(startedAt) {
			configChanged = true
			if verbose {
				ui.Info(fmt.Sprintf("Config newer than %s (config: %s, started: %s)",
					c.Name, configMtime.Format(time.RFC3339), startedAt.Format(time.RFC3339)))
			}
			break
		}
	}

	if configChanged && !skipBuild {
		ui.Info("Configuration changed since last start, rebuilding...")
		if _, err := runAutomaticBuild(workdir, build.BuildOptions{}); err != nil {
			ui.Error(fmt.Sprintf("Rebuild failed: %v", err))
			return fmt.Errorf("rebuilding: %w", err)
		}
		ui.Success("Rebuild complete")

		// Bring up with rebuilt config (--remove-orphans to clean up removed services)
		ui.Info("Applying updated configuration...")
		if err := comp.ComposeUp(ctx, workdir); err != nil {
			ui.Error(fmt.Sprintf("Failed to apply updated config: %v", err))
			return fmt.Errorf("applying updated config: %w", err)
		}
		ui.Success("Services restarted with updated configuration")
	} else {
		if configChanged && skipBuild {
			ui.Warn("Configuration changed but --skip-build specified, performing quick restart")
		} else if verbose {
			ui.Info("No configuration changes detected")
		}
		return restartQuick(ctx, comp, workdir, verbose)
	}

	return nil
}

// restartQuick performs a simple docker compose restart without rebuilding.
func restartQuick(ctx context.Context, comp *docker.Compose, workdir string, verbose bool) error {
	if verbose {
		ui.Info("Performing quick restart (no rebuild)")
	}
	ui.Info("Restarting services...")
	if err := comp.ComposeRestart(ctx, workdir); err != nil {
		ui.Error(fmt.Sprintf("Failed to restart: %v", err))
		return fmt.Errorf("restarting: %w", err)
	}
	ui.Success("Services restarted")
	return nil
}
