package commands

// deploy_run_bluegreen.go — blue/green canary deploy path for `nself deploy`.
//
// Purpose: handles the Y17 blue_green_deploy feature-flag path (--canary N /
//          --skip-canary with NSELF_FEATURE_BLUE_GREEN_DEPLOY=true), routing
//          to the bluegreen package. Split out of deploy_run.go
//          (T-P6-E2-W1-S1-T3) for 300-line compliance.
// Inputs:  the relevant runDeploy flag values + cmd/target/workdir.
// Outputs: handled=false when the blue/green path does not apply (canary flags
//          not set or the feature flag is off) — caller continues to the
//          legacy/pipeline path. handled=true means this function fully
//          handled the deploy and its err (possibly nil) is runDeploy's result.
// Constraints: pure move, same checks/output/errors, no behavior change.

import (
	"fmt"
	"os"
	"time"

	"github.com/nself-org/cli/internal/deploy/bluegreen"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// runDeployBlueGreenCanary handles the blue/green canary deploy path. See
// file header for the handled/err contract.
func runDeployBlueGreenCanary(cmd *cobra.Command, target, workdir string, canaryPct int, skipCanary, forceMigration, dryRun, force, jsonOut bool) (handled bool, err error) {
	bgEnabled := os.Getenv("NSELF_FEATURE_BLUE_GREEN_DEPLOY") == "true"
	if (canaryPct <= 0 && !skipCanary) || !bgEnabled {
		return false, nil
	}
	if !jsonOut {
		label := fmt.Sprintf("canary=%d%% skip-canary=%v force-migration=%v dry-run=%v", canaryPct, skipCanary, forceMigration, dryRun)
		ui.CommandHeader(fmt.Sprintf("nself deploy %s (blue/green)", target), label)
	}
	if target == "prod" && !dryRun && !force {
		return true, fmt.Errorf("production blue/green deploy requires --force. Re-run with --force once ready")
	}
	cfg := bluegreen.DeployConfig{
		ProjectRoot:    workdir,
		CanaryPercent:  canaryPct,
		SkipCanary:     skipCanary,
		ForceMigration: forceMigration,
		DryRun:         dryRun,
	}
	result := bluegreen.Deploy(cmd.Context(), cfg)
	if jsonOut && !streaming(cmd.Context()) {
		var runErr error
		if !result.Success {
			runErr = fmt.Errorf("%s", result.Error)
		}
		return true, emitDeployDoc("deploy", result, deployBlueGreenData(target, dryRun, result), runErr)
	}
	if result.RolledBack {
		ui.Error("Canary auto-rolled back: " + result.Error)
	} else if !result.Success {
		ui.Error("Blue/green deploy failed: " + result.Error)
	} else {
		ui.Success(fmt.Sprintf("Blue/green deploy complete in %s", result.Duration.Round(time.Millisecond)))
	}
	if !result.Success {
		return true, fmt.Errorf("%s", result.Error)
	}
	return true, nil
}
