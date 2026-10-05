// JSON data-type registration for the config domain (show, get, list).
//
// Purpose: config show/get/list answer with the v1 envelope in v1.5 mode only;
// their --json was accepted and ignored before P7-REG (EPIC D8).
// Constraints: registers from init(); see registry_types.go for the rules.

package commands

func init() {
	// config show: masked key/value map (secrets as "***" unless --reveal).
	registerJSONType("config show", map[string]string{})
	registerJSONType("config get", configGetData{})
	registerJSONType("config list", configListData{})
	for _, p := range []string{"config show", "config get", "config list"} {
		registerV15OnlyEnvelope(p)
	}
}
