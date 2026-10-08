package commands

// Purpose: choose the next safe command for bare nself in v1.5 mode.
// Inputs: a bounded context and the current project directory.
// Outputs: one Next line; detection never changes project or Docker state.
// Constraints: reuse the status health report and do not wait beyond the context.

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/health"
)

const nextStepTimeout = 3 * time.Second

// nextStep returns the one command a user should run next.
func nextStep(ctx context.Context) string {
	cwd, err := os.Getwd()
	if err != nil {
		return "Next: nself doctor"
	}
	root, err := config.FindNSelfRoot(cwd)
	if err != nil {
		return "Next: nself init"
	}
	if _, err := os.Stat(filepath.Join(root, "docker-compose.yml")); err != nil {
		return "Next: nself start"
	}
	// The Docker funnel's process group handling keeps a slow daemon probe
	// within the shared deadline, including child processes of a Docker stub.
	if err := docker.NewCompose().Run(ctx, root, "info", "--format", "{{.ServerVersion}}"); err != nil {
		return "Next: nself doctor"
	}
	cfg, err := config.Load(root)
	if err != nil {
		return "Next: nself doctor"
	}
	report, err := health.RunAllChecks(ctx, cfg, root)
	if err != nil || ctx.Err() != nil {
		return "Next: nself doctor"
	}
	if report.Total == 0 {
		return "Next: nself start"
	}
	missing := 0
	for _, result := range report.Results {
		if result.Status == "not_found" {
			missing++
		}
	}
	if missing == report.Total {
		return "Next: nself start"
	}
	switch statusState(report.Results) {
	case stateOK, stateTransitional:
		return "Next: nself status"
	default:
		return "Next: nself doctor"
	}
}
