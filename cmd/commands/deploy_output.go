package commands

// Purpose: shared result types (deployStep, deployResult) and the formatting
// helpers (stepStatus, finalize) used to print or json-encode a deploy run's
// outcome. Inputs are the collected steps and an optional error; outputs are
// printed text/JSON and the final error to return from RunE.
// Constraints: split out of deploy.go (CLI-R12) as a pure move, no behavior change.

import (
	"fmt"
	"time"

	"github.com/nself-org/cli/internal/ui"
)

type deployStep struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type deployResult struct {
	Target   string       `json:"target"`
	Strategy string       `json:"strategy"`
	Steps    []deployStep `json:"steps"`
	Duration int64        `json:"durationMs"`
	Success  bool         `json:"success"`
	Error    string       `json:"error,omitempty"`
}

func stepStatus(dryRun bool, ok string) string {
	if dryRun {
		return "pending"
	}
	return ok
}

func finalize(jsonOut bool, target, strategy string, start time.Time, steps []deployStep, err error) error {
	duration := time.Since(start).Milliseconds()
	success := err == nil
	if jsonOut {
		res := deployResult{
			Target:   target,
			Strategy: strategy,
			Steps:    steps,
			Duration: duration,
			Success:  success,
		}
		if err != nil {
			res.Error = err.Error()
		}
		return emitDeployDoc("deploy", res, deployDataOf(res), err)
	}
	if success {
		ui.Success(fmt.Sprintf("Deploy %s (%s) finished in %dms", target, strategy, duration))
	} else {
		ui.Error(fmt.Sprintf("Deploy %s (%s) failed after %dms: %v", target, strategy, duration, err))
	}
	return err
}
