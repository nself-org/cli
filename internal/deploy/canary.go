// Package deploy provides deployment management for the nSelf CLI.
// This file implements the canary deploy strategy adapter (T06).
//
// Canary deploys:
//   - Spin up a canary (green) stack alongside the live (blue) stack.
//   - Route N% of traffic to green via Nginx weighted upstream.
//   - Monitor health during a configurable soak window (default 300s / 5min).
//   - On healthy soak: gradually shift traffic 10% → 50% → 100%.
//   - On unhealthy soak: revert canary to 0%, tear down green stack, log.
//
// This file is the strategy-path adapter called when the user passes
// --strategy=canary. It delegates all orchestration to the internal
// bluegreen package and writes outcome to .nself/state/deploy.json.
package deploy

import (
	"context"
	"time"

	"github.com/nself-org/cli/internal/deploy/bluegreen"
	"github.com/nself-org/cli/internal/ui"
)

// CanaryConfig holds the parameters for a canary deploy.
type CanaryConfig struct {
	// ProjectRoot is the nSelf project root (contains docker-compose.yml).
	ProjectRoot string

	// Target is the deploy target: local, staging, prod.
	Target string

	// InitialPercent is the initial canary traffic percentage (1-99).
	// Defaults to 10 when zero.
	InitialPercent int

	// SoakMinutes is the canary soak period in minutes. Defaults to 5.
	SoakMinutes int

	// ErrorThresholdPct is the error rate percentage that triggers auto-rollback.
	// Defaults to 1.0.
	ErrorThresholdPct float64

	// HealthTimeoutSec overrides the default 30-second health check timeout.
	// Zero means use the default.
	HealthTimeoutSec int

	// ForceMigration disables the migration compatibility check.
	// Required when a migration is not backward-compatible with canary traffic.
	ForceMigration bool

	// DryRun prints steps without executing them.
	DryRun bool
}

// CanaryResult is the outcome of a canary deploy.
type CanaryResult struct {
	// Success is true when the deploy completed without error.
	Success bool

	// Steps is the ordered list of deploy steps with their status.
	Steps []bluegreen.DeployStep

	// Duration is how long the deploy took.
	Duration time.Duration

	// FinalPercent is the final canary traffic percentage (100 on success, 0 on rollback).
	FinalPercent int

	// RolledBack is true when an auto-rollback was triggered during the soak.
	RolledBack bool

	// Error is set on failure.
	Error string
}

// RunCanary executes a canary deploy:
//  1. Spins up the canary (green) stack alongside blue.
//  2. Health-checks green.
//  3. Routes InitialPercent of traffic to green via Nginx weighted upstream.
//  4. Monitors green health during the soak window.
//  5. On healthy: promotes green to 100% traffic, tears down blue.
//  6. On unhealthy: reverts to 0% canary, tears down green, returns error.
//  7. Persists state to .nself/state/deploy.json.
func RunCanary(ctx context.Context, cfg CanaryConfig) CanaryResult {
	start := time.Now()

	initialPct := cfg.InitialPercent
	if initialPct <= 0 {
		initialPct = 10 // spec default
	}

	bgCfg := bluegreen.DeployConfig{
		ProjectRoot:       cfg.ProjectRoot,
		CanaryPercent:     initialPct,
		SoakMinutes:       cfg.SoakMinutes,
		ErrorThresholdPct: cfg.ErrorThresholdPct,
		HealthTimeoutSec:  cfg.HealthTimeoutSec,
		ForceMigration:    cfg.ForceMigration,
		DryRun:            cfg.DryRun,
		SkipCanary:        false, // explicit: canary soak is the whole point
	}

	result := bluegreen.Deploy(ctx, bgCfg)

	// Persist state to .nself/state/deploy.json regardless of outcome.
	stateErr := cfg.saveDeployState(start, result)

	if stateErr != nil && result.Success {
		// Non-fatal: bluegreen package already wrote its own state file.
		ui.Warn("deploy state write failed (non-fatal): " + stateErr.Error())
	}

	return CanaryResult{
		Success:      result.Success,
		Steps:        result.Steps,
		Duration:     result.Duration,
		FinalPercent: result.CanaryPercent,
		RolledBack:   result.RolledBack,
		Error:        result.Error,
	}
}

// saveDeployState writes the outcome of a canary deploy to the unified state file.
func (cfg CanaryConfig) saveDeployState(start time.Time, result bluegreen.DeployResult) error {
	state := StrategyState{
		Strategy:           StrategyCanary,
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
