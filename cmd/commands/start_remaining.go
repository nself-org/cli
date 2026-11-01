package commands

// Purpose: reconcile remaining services after PostgreSQL initialization.
// Inputs: the Compose client, project directory and loaded config.
// Outputs: running services, then best-effort background container cleanup.
// Constraints: Compose reads project values from env files, never the cascade
// exported into the CLI process; restore env before spawning background work.

import (
	"context"
	"fmt"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/ui"
)

func startRemainingServicesPhase(ctx context.Context, compose *docker.Compose, projectDir string, cfg *config.Config) error {
	svcSp := ui.NewSpinner("Running docker compose up...")
	svcSp.Start()
	if err := config.WithComposeEnvironment(func() error { return compose.ComposeUp(ctx, projectDir) }); err != nil {
		svcSp.Fail("Docker compose up failed")
		ui.UXError(
			"Failed to start services",
			err.Error(),
			[]string{
				"Check Docker is running: docker info",
				"Review logs: nself logs",
				"Try a clean start: nself start --clean-start",
				"Rebuild: nself build --force",
			},
		)
		return fmt.Errorf("compose up: %w", err)
	}
	svcSp.Success("All services started")
	docker.StartPostStartCleanup(cfg.ProjectName)
	return nil
}
