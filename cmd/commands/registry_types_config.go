// JSON data-type registration for the config domain (show, get, list, set).
//
// Purpose: config show/get/list answer with the v1 envelope in v1.5 mode only;
// their --json was accepted and ignored before P7-REG (EPIC D8). config set
// (P7-LIVE-05) answers with configSetData: in v1.5 mode after an apply, and in
// both modes with --preview (a new flag, nothing to stay compatible with).
// Constraints: registers from init(); see registry_types.go for the rules.

package commands

import "github.com/nself-org/cli/internal/reconcile"

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
}
