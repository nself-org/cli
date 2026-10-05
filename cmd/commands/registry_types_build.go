// JSON data-type registration for the build domain (build).
//
// Purpose: `nself build --json` answers with the v1 envelope whose data is the
// change plan (contract:cli.change-plan v1): the plan `--plan` shows, or the
// plan that was applied. One data type for the command (REG D1). The flags are
// additive in both compat modes, so the envelope is not v1.5-only.
// Constraints: registers from init(); see registry_types.go for the rules.

package commands

import "github.com/nself-org/cli/internal/reconcile"

func init() {
	registerJSONType("build", reconcile.Plan{})
}
