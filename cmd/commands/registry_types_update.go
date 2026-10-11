// JSON data-type registration for `update` (P7-LIVE-10).
//
// Purpose: `nself update --plan --json` and `nself update --yes --json` answer
// with the v1 envelope whose data is UpdateResult: the plan (pin diff plus the
// ordered steps), whether it was applied, and the status of each step. One data
// type for the command (REG D1). The flags are additive in both compat modes, so
// the envelope is not v1.5-only; a plain `nself update --json` is still
// refused with E402 (update.go: runUpdatePlanMode).
// Constraints: registers from init(); see registry_types.go for the rules.
// plan.Plan is the shared step model that `update stack` (P7-PLUG-31) runs.

package commands

import "github.com/nself-org/cli/internal/upgrade/plan"

// UpdateStep is one step of an upgrade and what happened to it.
type UpdateStep struct {
	Name   string `json:"name"`
	Status string `json:"status"` // pending (plan) | done | skipped
	Detail string `json:"detail,omitempty"`
}

// UpdateResult is the envelope data of `update --plan|--yes --json`.
type UpdateResult struct {
	Plan    plan.Plan    `json:"plan"`
	Applied bool         `json:"applied"` // false under --plan
	Steps   []UpdateStep `json:"steps"`
}

func init() { registerJSONType("update", UpdateResult{}) }
