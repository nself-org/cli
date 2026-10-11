// JSON data-type registration for the config domain (show, get, list, set, and
// P7-SURF-12: env, export, features, import, oauth, secrets, telemetry, trust,
// validate).
//
// Purpose: config show/get/list answer with the v1 envelope in v1.5 mode only;
// their --json was accepted and ignored before P7-REG (EPIC D8). config set
// (P7-LIVE-05) answers with configSetData in v1.5 mode (an apply or a
// --preview); in v1.4 mode its --json stays accepted and ignored.
// Constraints: registers from init(); see registry_types.go for the rules.

package commands

import (
	"github.com/nself-org/cli/internal/featureflags"
	"github.com/nself-org/cli/internal/reconcile"
)

// configSetData is the data of `nself config set --json` (P7-LIVE-05).
type configSetData struct {
	// Key is the config key that was set (or previewed).
	Key string `json:"key"`
	// File is the env file, relative to the project (".env", ".env.secrets").
	File string `json:"file"`
	// Applied is true when the line was written and the plan applied; false
	// for --preview, which writes nothing.
	Applied bool `json:"applied"`
	// Plan is the change plan of the regeneration (contract:cli.change-plan).
	Plan reconcile.Plan `json:"plan"`
}

func init() {
	// config show: masked key/value map (secrets as "***" unless --reveal).
	registerJSONType("config show", map[string]string{})
	registerJSONType("config get", configGetData{})
	registerJSONType("config list", configListData{})
	registerJSONType("config set", configSetData{})
	for _, p := range []string{"config show", "config get", "config list", "config set"} {
		registerV15OnlyEnvelope(p)
	}
	for path, zero := range configEnvelopeTypes {
		registerJSONType(path, zero)
		registerV15OnlyEnvelope(path)
	}
}

// configEnvelopeTypes maps each remaining path of the config fragment to its
// envelope data (P7-SURF-12). Command groups answer with their subcommands
// (dataHub); read commands with typed facts (config_envelope_types.go);
// write, remote and destructive commands with the shared dataResult. Rows
// that printed JSON before the contract keep that bare output in v1.4 and say
// so in the v1.4 registry view (`json: legacy` in config.yaml): env target
// list and probe, features list and status, export --format json.
// `config secrets edit` is interactive and registers nothing.
var configEnvelopeTypes = map[string]any{
	"config":                           dataHub{},
	"config env":                       dataHub{},
	"config env copy":                  dataResult{},
	"config env diff":                  envDiffData{},
	"config env explain":               envExplainData{},
	"config env list":                  envListData{},
	"config env show":                  envShowData{},
	"config env target":                dataHub{},
	"config env target add":            dataResult{},
	"config env target list":           []envTargetListRow{},
	"config env target migrate":        dataResult{},
	"config env target probe":          []probeRow{},
	"config env target remove":         dataResult{},
	"config env use":                   dataResult{},
	"config export":                    map[string]string{},
	"config features":                  dataHub{},
	"config features disable":          dataResult{},
	"config features enable":           dataResult{},
	"config features list":             []featureflags.Status{},
	"config features status":           featureflags.Status{},
	"config import":                    dataResult{},
	"config oauth":                     dataHub{},
	"config oauth refresh":             dataResult{},
	"config secrets":                   dataHub{},
	"config secrets audit":             secretsAuditData{},
	"config secrets decrypt-on-deploy": secretsDecryptData{},
	"config secrets get":               secretsGetData{},
	"config secrets init":              dataResult{},
	"config secrets lint":              secretsLintData{},
	"config secrets list":              secretsListData{},
	"config secrets list-schedules":    secretsSchedulesData{},
	"config secrets rekey":             dataResult{},
	"config secrets retire":            dataResult{},
	"config secrets rotate":            dataResult{},
	"config secrets rotation-log":      secretsRotationLogData{},
	"config secrets schedule":          secretsScheduleData{},
	"config secrets set":               dataResult{},
	"config secrets verify":            dataResult{},
	"config telemetry":                 dataHub{},
	"config telemetry off":             dataResult{},
	"config telemetry on":              dataResult{},
	"config telemetry status":          telemetryStatusData{},
	"config trust":                     dataResult{},
	"config trust dns":                 dataResult{},
	"config trust ssl":                 dataHub{},
	"config trust ssl add":             dataResult{},
	"config trust ssl renew":           dataResult{},
	"config trust ssl setup":           dataResult{},
	"config trust ssl status":          dataResult{},
	"config trust status":              trustStatusData{},
	"config validate":                  configValidateData{},
}
