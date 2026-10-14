// JSON data-type registration for the services canon fragment (`config
// services` and its subcommands; `service ...` is the v1.4 spelling).
//
// Purpose: every runnable document command of the fragment answers with the v1
// envelope in v1.5 mode (P7-SURF-13). The group answers with its subcommands
// (dataHub); list, ps, enable and disable with typed facts
// (service_envelope.go); the writing and lifecycle commands with the shared
// dataResult. `config services list` printed a JSON array before the contract:
// that array stays in v1.4 mode and `json: legacy` in services.yaml says so for
// the v1.4 registry view; the v1.5 envelope carries the same array.
// Constraints: registers from init(); see registry_types.go for the rules.

package commands

// servicesEnvelopeTypes maps each path of the fragment to its envelope data.
var servicesEnvelopeTypes = map[string]any{
	"config services":           dataHub{},
	"config services add":       dataResult{},
	"config services configure": dataResult{},
	"config services disable":   serviceToggleData{},
	"config services enable":    serviceToggleData{},
	"config services list":      serviceListData{},
	"config services ps":        servicePsData{},
	"config services restart":   dataResult{},
	"config services scale":     dataResult{},
	"config services start":     dataResult{},
	"config services stop":      dataResult{},
	"config services update":    dataResult{},
	"config services upgrade":   dataResult{},
}

func init() {
	for path, zero := range servicesEnvelopeTypes {
		registerJSONType(path, zero)
		registerV15OnlyEnvelope(path)
	}
}
