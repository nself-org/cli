package promote

// Purpose: Bind promotion recovery to the deploy operation it applied.
// Inputs: Local deploy state and promotion state. Outputs: Immutable identity.
// Constraints: Completion timestamps never order operations; legacy records use
// their per-host tar identity. Existing record identities never change on save.

import (
	"maps"
	"time"

	"github.com/nself-org/cli/internal/deploy"
)

// DeployIdentity records the deploy.json identity written by promotion apply.
type DeployIdentity struct {
	StartedAt     time.Time         `json:"started_at"`
	PreDeployTar  string            `json:"pre_deploy_tar,omitempty"`
	PreDeployTars map[string]string `json:"pre_deploy_tars,omitempty"`
}

// Purpose: Capture the applied deploy identity once, preserving it on retries.
// Inputs: Project and record. Outputs: Bound record or state-read error.
// Constraints: Only matching applied tars authorize a new identity binding.
func captureDeployIdentity(root string, rec *Record) error {
	previous, err := LoadRecord(root)
	if err != nil {
		return err
	}
	if previous != nil && previous.ID == rec.ID && previous.DeployIdentity != nil {
		rec.DeployIdentity = previous.DeployIdentity
		return nil
	}
	state, err := deploy.LoadDeployState(root)
	if err != nil {
		return err
	}
	if state.Target == rec.Target && recordTarMatchesDeploy(rec, state) {
		rec.DeployIdentity = &DeployIdentity{StartedAt: state.StartedAt, PreDeployTar: state.PreDeployTar, PreDeployTars: state.PreDeployTars}
	}
	return nil
}

// Purpose: Determine whether deploy.json still describes this promotion.
// Inputs: Recorded identity and deploy state. Outputs: Identity equality.
// Constraints: Older records without an identity fall back to their tar lists.
func recordMatchesDeploy(rec *Record, state deploy.StrategyState) bool {
	if rec.DeployIdentity == nil {
		return recordTarMatchesDeploy(rec, state)
	}
	id := rec.DeployIdentity
	return id.StartedAt.Equal(state.StartedAt) && id.PreDeployTar == state.PreDeployTar && maps.Equal(id.PreDeployTars, state.PreDeployTars)
}

// Purpose: Match legacy promotion authority to the deployment's artifact backup.
// Inputs: Per-host saved tars and deploy state. Outputs: Tar identity equality.
// Constraints: The single-tar legacy form is valid only for one recorded host.
func recordTarMatchesDeploy(rec *Record, state deploy.StrategyState) bool {
	if len(state.PreDeployTars) > 0 {
		return maps.Equal(rec.PreDeployTar, state.PreDeployTars)
	}
	if len(rec.PreDeployTar) == 1 {
		for _, tar := range rec.PreDeployTar {
			return tar != "" && tar == state.PreDeployTar
		}
	}
	return false
}
