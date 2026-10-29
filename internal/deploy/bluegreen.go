// Package deploy provides deployment management for the nSelf CLI.
// This file implements the blue-green deploy strategy adapter (T06).
//
// Blue-green deploys:
//   - Spin up a "green" stack alongside the live "blue" stack.
//   - Run health checks on green.
//   - Flip Nginx upstream weights from blue → green (100% flip, no canary phase).
//   - Drain blue connections (60s grace via the bluegreen package).
//   - Tear down blue.
//   - On green failure: abort, keep blue running, log error.
//
// This file is the strategy-path adapter called when the user passes
// --strategy=blue-green. It delegates all orchestration to the internal
// bluegreen package and writes outcome to .nself/state/deploy.json.
package deploy

import (
	"context"
	"time"

	"github.com/nself-org/cli/internal/deploy/bluegreen"
	"github.com/nself-org/cli/internal/ui"
)

// BlueGreenConfig holds the parameters for a blue-green deploy.
type BlueGreenConfig struct {
	// ProjectRoot is the nSelf project root (contains docker-compose.yml).
	ProjectRoot string

	// Target is the deploy target: local, staging, prod.
	Target string

	// DryRun prints steps without executing them.
	DryRun bool

	// HealthTimeoutSec overrides the default 30-second health check timeout.
	// Zero means use the default.
	HealthTimeoutSec int

	// ForceMigration disables the migration compatibility check and performs a
	// full blue-green flip even when backward-incompatible migrations are pending.
	ForceMigration bool
}

// BlueGreenResult is the outcome of a blue-green deploy.
type BlueGreenResult struct {
	// Success is true when the deploy completed without error.
	Success bool

	// Steps is the ordered list of deploy steps with their status.
	Steps []bluegreen.DeployStep

	// Duration is how long the deploy took.
	Duration time.Duration

	// RolledBack is true when an automatic rollback was triggered.
	RolledBack bool

	// Error is set on failure.
	Error string
}

// RunBlueGreen executes a blue-green deploy:
//  1. Spins up the green stack alongside blue.
//  2. Health-checks green.
//  3. Flips Nginx upstream to 100% green (no canary phase).
//  4. Health-checks green at 100%.
//  5. Stops blue.
//  6. Persists state to .nself/state/deploy.json.
//
// On green failure: aborts immediately, keeps blue running, returns error.
func RunBlueGreen(ctx context.Context, cfg BlueGreenConfig) BlueGreenResult {
	start := time.Now()

	bgCfg := bluegreen.DeployConfig{
		ProjectRoot:      cfg.ProjectRoot,
		SkipCanary:       true, // blue-green = full flip, no canary soak
		ForceMigration:   cfg.ForceMigration,
		DryRun:           cfg.DryRun,
		HealthTimeoutSec: cfg.HealthTimeoutSec,
	}

	result := bluegreen.Deploy(ctx, bgCfg)

	// Persist state to .nself/state/deploy.json regardless of outcome.
	stateErr := cfg.saveDeployState(start, result)

	if stateErr != nil && result.Success {
		// State persistence failure is non-fatal when the deploy succeeded.
		// The bluegreen package's own state file (.nself/bluegreen/state.json) is
		// already written; log the mismatch for diagnostics.
		ui.Warn("deploy state write failed (non-fatal): " + stateErr.Error())
	}

	return BlueGreenResult{
		Success:    result.Success,
		Steps:      result.Steps,
		Duration:   result.Duration,
		RolledBack: result.RolledBack,
		Error:      result.Error,
	}
}

// saveDeployState writes the outcome of a blue-green deploy to the unified state file.
func (cfg BlueGreenConfig) saveDeployState(start time.Time, result bluegreen.DeployResult) error {
	state := StrategyState{
		Strategy:           StrategyBlueGreen,
		Target:             cfg.Target,
		StartedAt:          start,
		CompletedAt:        time.Now().UTC(),
		Success:            result.Success,
		CanaryPercent:      result.CanaryPercent,
		RolledBack:         result.RolledBack,
		BlueGreenStateFile: bluegreen.StateFile,
	}
	if !result.Success {
		state.Error = result.Error
	}
	return SaveDeployState(cfg.ProjectRoot, state)
}
