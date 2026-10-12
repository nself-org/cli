// JSON data-type registration for the deploy canon fragment (deploy, deploy
// status|health|check-access|environments|access|ops|web|rollback|promote|
// promote-env|security|targets, doctor security, update images|upgrade|
// verify-sbom).
//
// Purpose: every runnable document command of the fragment answers with the v1
// envelope in v1.5 mode (P7-SURF-15; `deploy targets` is P7-DEPL-13's and
// `deploy promote` P7-DEPL-15's). Command groups answer with their subcommands
// (dataHub); read commands with typed facts (deploy_envelope_types.go); write,
// remote and destructive commands, and reads whose facts only a child process
// holds, with the shared dataResult. `deploy promote-env` is the v1.4 spelling
// of `deploy promote` and answers with the promote engine's Result.
// Commands that printed JSON before the contract keep that output in v1.4 (the
// cmdregistry mapping keeps `json: legacy` for V15-only rows). `deploy logs` is
// a stream and registers nothing.
// Constraints: registers from init(); see registry_types.go for the rules.

package commands

import cpromote "github.com/nself-org/cli/internal/controlplane/promote"

// deployEnvelopeTypes maps each v1.5-only path of the fragment to its data.
var deployEnvelopeTypes = map[string]any{
	"deploy":                      deployData{},
	"deploy access":               dataHub{},
	"deploy access grant":         dataResult{},
	"deploy access list":          accessListData{},
	"deploy access revoke":        dataResult{},
	"deploy check-access":         deployCheckAccessData{},
	"deploy environments":         deployEnvironmentsOutput{},
	"deploy health":               dataResult{},
	"deploy ops":                  dataResult{},
	"deploy promote":              cpromote.Result{},
	"deploy promote-env":          cpromote.Result{},
	"deploy promote-env rollback": dataResult{},
	"deploy rollback":             dataResult{},
	"deploy security setup":       securitySetupData{},
	"deploy status":               deployStatusData{},
	"deploy web":                  dataResult{},
	"doctor security audit":       securityAuditData{},
	"doctor security status":      securityStatusData{},
	"update images":               updateImagesData{},
	"update upgrade":              updateUpgradeData{},
	"update verify-sbom":          verifySBOMData{},
}

func init() {
	registerJSONType("deploy targets", deployTargetsData{})
	for path, zero := range deployEnvelopeTypes {
		registerJSONType(path, zero)
		registerV15OnlyEnvelope(path)
	}
}
