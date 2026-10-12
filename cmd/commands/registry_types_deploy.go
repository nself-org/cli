package commands

import cpromote "github.com/nself-org/cli/internal/controlplane/promote"

func init() {
	registerJSONType("deploy targets", deployTargetsData{})
	registerJSONType("deploy promote", cpromote.Result{})
	registerV15OnlyEnvelope("deploy promote")
}
